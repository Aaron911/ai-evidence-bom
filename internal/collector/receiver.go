package collector

import (
	"compress/gzip"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/aggregate"
	"github.com/Aaron911/ai-evidence-bom/internal/cyclonedx"
	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
	"github.com/Aaron911/ai-evidence-bom/internal/normalize"
)

const (
	DefaultMaxRequestBytes = 64 << 20
	DefaultMaxDedupeItems  = 100_000
)

var errRequestTooLarge = errors.New("request body exceeds configured limit")

type Config struct {
	GraphOut         string
	BOMOut           string
	Source           string
	AuthToken        string
	SensitiveHMACKey []byte
	MaxRequestBytes  int64
	MaxDedupeItems   int
	Now              func() time.Time
}

type Stats struct {
	Requests       uint64    `json:"requests"`
	ReceivedSpans  uint64    `json:"receivedSpans"`
	AcceptedSpans  uint64    `json:"acceptedSpans"`
	DuplicateSpans uint64    `json:"duplicateSpans"`
	FailedRequests uint64    `json:"failedRequests"`
	LastAcceptedAt time.Time `json:"lastAcceptedAt,omitempty"`
}

// Receiver accepts OTLP/HTTP JSON traces and continuously materializes an
// evidence graph. It intentionally does not retain raw payloads.
type Receiver struct {
	config Config

	mu        sync.Mutex
	graph     model.Graph
	seen      map[string]struct{}
	seenOrder []string
	stats     Stats
}

func New(config Config) (*Receiver, error) {
	if strings.TrimSpace(config.GraphOut) == "" {
		return nil, errors.New("graph output path is required")
	}
	if config.MaxRequestBytes == 0 {
		config.MaxRequestBytes = DefaultMaxRequestBytes
	}
	if config.MaxRequestBytes < 1 {
		return nil, errors.New("max request bytes must be positive")
	}
	if config.MaxDedupeItems == 0 {
		config.MaxDedupeItems = DefaultMaxDedupeItems
	}
	if config.MaxDedupeItems < 1 {
		return nil, errors.New("max dedupe items must be positive")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Source == "" {
		config.Source = "otlp-http"
	}
	receiver := &Receiver{
		config: config,
		seen:   make(map[string]struct{}),
		graph: model.Graph{
			SchemaVersion: model.SchemaVersion,
			Source:        config.Source,
			Nodes:         []model.Node{},
			Edges:         []model.Edge{},
			Metadata: map[string]string{
				"privacy.mode":       "metadata-only",
				"normalizer.version": model.SchemaVersion,
				"aggregation.mode":   "continuous",
			},
		},
	}
	if err := receiver.loadExistingGraph(); err != nil {
		return nil, err
	}
	return receiver, nil
}

func (receiver *Receiver) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/healthz":
		receiver.handleHealth(response, request)
	case "/v1/traces":
		receiver.handleTraces(response, request)
	case "/v1/evidence":
		receiver.handleEvidence(response, request)
	case "/v1/bom":
		receiver.handleBOM(response, request)
	case "/v1/stats":
		receiver.handleStats(response, request)
	default:
		writeStatus(response, http.StatusNotFound, "endpoint not found")
	}
}

func (receiver *Receiver) handleHealth(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(response, http.MethodGet)
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{
		"status":        "ok",
		"schemaVersion": model.SchemaVersion,
	})
}

func (receiver *Receiver) handleTraces(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(response, http.MethodPost)
		return
	}
	if !receiver.authorized(request) {
		response.Header().Set("WWW-Authenticate", "Bearer")
		writeStatus(response, http.StatusUnauthorized, "missing or invalid bearer token")
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeStatus(response, http.StatusUnsupportedMediaType, "only OTLP/HTTP JSON is supported; use Content-Type: application/json")
		return
	}
	body, err := readBody(response, request, receiver.config.MaxRequestBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			writeStatus(response, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		writeStatus(response, http.StatusBadRequest, err.Error())
		return
	}
	observations, source, err := inputpkg.ParseOTLP(body, receiver.config.Source)
	if err != nil {
		receiver.recordFailure()
		writeStatus(response, http.StatusBadRequest, err.Error())
		return
	}
	if err := receiver.accept(observations, source); err != nil {
		receiver.recordFailure()
		writeStatus(response, http.StatusInternalServerError, "persist evidence snapshot: "+err.Error())
		return
	}
	writeJSON(response, http.StatusOK, struct{}{})
}

func (receiver *Receiver) handleEvidence(response http.ResponseWriter, request *http.Request) {
	if !receiver.authorizedRead(response, request) {
		return
	}
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	writeJSON(response, http.StatusOK, receiver.graph)
}

func (receiver *Receiver) handleBOM(response http.ResponseWriter, request *http.Request) {
	if !receiver.authorizedRead(response, request) {
		return
	}
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	writeJSON(response, http.StatusOK, cyclonedx.Export(receiver.graph))
}

func (receiver *Receiver) handleStats(response http.ResponseWriter, request *http.Request) {
	if !receiver.authorizedRead(response, request) {
		return
	}
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	writeJSON(response, http.StatusOK, receiver.stats)
}

func (receiver *Receiver) authorizedRead(response http.ResponseWriter, request *http.Request) bool {
	if request.Method != http.MethodGet {
		methodNotAllowed(response, http.MethodGet)
		return false
	}
	if !receiver.authorized(request) {
		response.Header().Set("WWW-Authenticate", "Bearer")
		writeStatus(response, http.StatusUnauthorized, "missing or invalid bearer token")
		return false
	}
	return true
}

func (receiver *Receiver) authorized(request *http.Request) bool {
	if receiver.config.AuthToken == "" {
		return true
	}
	parts := strings.SplitN(strings.TrimSpace(request.Header.Get("Authorization")), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	provided := strings.TrimSpace(parts[1])
	if len(provided) != len(receiver.config.AuthToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(receiver.config.AuthToken)) == 1
}

func (receiver *Receiver) accept(observations []inputpkg.Observation, source string) error {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()

	receiver.stats.Requests++
	receiver.stats.ReceivedSpans += uint64(len(observations))
	unique := make([]inputpkg.Observation, 0, len(observations))
	for _, observation := range observations {
		key := observationKey(observation)
		if key != "" {
			if _, exists := receiver.seen[key]; exists {
				receiver.stats.DuplicateSpans++
				continue
			}
			receiver.seen[key] = struct{}{}
			receiver.seenOrder = append(receiver.seenOrder, key)
		}
		unique = append(unique, observation)
	}
	receiver.trimDedupe()

	now := receiver.config.Now().UTC()
	if len(unique) > 0 {
		batch := normalize.BuildWithOptions(unique, source, now, normalize.Options{
			SensitiveHMACKey: receiver.config.SensitiveHMACKey,
		})
		receiver.graph = aggregate.Merge(receiver.graph, batch, now)
		receiver.stats.AcceptedSpans += uint64(len(unique))
		receiver.stats.LastAcceptedAt = now
	}
	if receiver.graph.GeneratedAt.IsZero() {
		receiver.graph.GeneratedAt = now
	}
	return receiver.persist()
}

func (receiver *Receiver) trimDedupe() {
	overflow := len(receiver.seenOrder) - receiver.config.MaxDedupeItems
	if overflow <= 0 {
		return
	}
	for _, key := range receiver.seenOrder[:overflow] {
		delete(receiver.seen, key)
	}
	copy(receiver.seenOrder, receiver.seenOrder[overflow:])
	receiver.seenOrder = receiver.seenOrder[:len(receiver.seenOrder)-overflow]
}

func (receiver *Receiver) persist() error {
	if err := writeFileAtomic(receiver.config.GraphOut, receiver.graph, 0o644); err != nil {
		return err
	}
	if receiver.config.BOMOut != "" {
		if err := writeFileAtomic(receiver.config.BOMOut, cyclonedx.Export(receiver.graph), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (receiver *Receiver) loadExistingGraph() error {
	data, err := os.ReadFile(receiver.config.GraphOut)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read existing graph: %w", err)
	}
	if err := json.Unmarshal(data, &receiver.graph); err != nil {
		return fmt.Errorf("decode existing graph: %w", err)
	}
	if receiver.graph.SchemaVersion == "" {
		return errors.New("existing graph has no schemaVersion")
	}
	receiver.graph.Canonicalize()
	return nil
}

func (receiver *Receiver) recordFailure() {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	receiver.stats.FailedRequests++
}

func observationKey(observation inputpkg.Observation) string {
	if observation.TraceID == "" || observation.SpanID == "" {
		return ""
	}
	return strings.ToLower(observation.TraceID) + "\x00" + strings.ToLower(observation.SpanID)
}

func readBody(response http.ResponseWriter, request *http.Request, limit int64) ([]byte, error) {
	reader := io.Reader(http.MaxBytesReader(response, request.Body, limit+1))
	encoding := strings.TrimSpace(strings.ToLower(request.Header.Get("Content-Encoding")))
	switch encoding {
	case "", "identity":
	case "gzip":
		compressed, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("decode gzip request: %w", err)
		}
		defer compressed.Close()
		reader = compressed
	default:
		return nil, fmt.Errorf("unsupported Content-Encoding %q", encoding)
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return nil, errRequestTooLarge
		}
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, errRequestTooLarge
	}
	return data, nil
}

func writeFileAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil && directory != "." {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".aiebom-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary snapshot: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return fmt.Errorf("set snapshot permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace snapshot: %w", err)
	}
	return nil
}

func methodNotAllowed(response http.ResponseWriter, allowed string) {
	response.Header().Set("Allow", allowed)
	writeStatus(response, http.StatusMethodNotAllowed, "method not allowed")
}

func writeStatus(response http.ResponseWriter, status int, message string) {
	writeJSON(response, status, map[string]string{"message": message})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
