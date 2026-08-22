package collector

import (
	"compress/gzip"
	"context"
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

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Aaron911/ai-evidence-bom/internal/aggregate"
	"github.com/Aaron911/ai-evidence-bom/internal/cyclonedx"
	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
	"github.com/Aaron911/ai-evidence-bom/internal/normalize"
	"github.com/Aaron911/ai-evidence-bom/internal/trust"
)

const (
	DefaultMaxRequestBytes = 64 << 20
	DefaultMaxDedupeItems  = 100_000
	mediaTypeJSON          = "application/json"
	mediaTypeProtobuf      = "application/x-protobuf"
)

var errRequestTooLarge = errors.New("request body exceeds configured limit")

type Config struct {
	GraphOut         string
	BOMOut           string
	Source           string
	AuthToken        string
	SensitiveHMACKey []byte
	SourceTrust      trust.Policy
	MaxRequestBytes  int64
	MaxDedupeItems   int
	Now              func() time.Time
}

type Stats struct {
	Requests           uint64    `json:"requests"`
	ReceivedSpans      uint64    `json:"receivedSpans"`
	AcceptedSpans      uint64    `json:"acceptedSpans"`
	DuplicateSpans     uint64    `json:"duplicateSpans"`
	EvidenceDowngrades uint64    `json:"evidenceDowngrades"`
	PendingSpans       uint64    `json:"pendingSpans"`
	FailedRequests     uint64    `json:"failedRequests"`
	LastAcceptedAt     time.Time `json:"lastAcceptedAt,omitempty"`
}

// Receiver accepts OTLP/HTTP and OTLP/gRPC traces and continuously
// materializes an evidence graph. It intentionally does not retain raw payloads.
type Receiver struct {
	collectortracepb.UnimplementedTraceServiceServer

	config Config

	mu        sync.Mutex
	graph     model.Graph
	seen      map[string]struct{}
	seenOrder []string
	contexts  map[string]normalize.AgentContext
	pending   []inputpkg.Observation
	stats     Stats
}

var _ collectortracepb.TraceServiceServer = (*Receiver)(nil)

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
	if err := config.SourceTrust.Validate(); err != nil {
		return nil, fmt.Errorf("validate source trust policy: %w", err)
	}
	if config.Source == "" {
		config.Source = "otlp-http"
	}
	receiver := &Receiver{
		config:   config,
		seen:     make(map[string]struct{}),
		contexts: make(map[string]normalize.AgentContext),
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
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != mediaTypeJSON && mediaType != mediaTypeProtobuf {
		writeStatus(response, http.StatusUnsupportedMediaType, "supported Content-Type values are application/json and application/x-protobuf")
		return
	}
	if !receiver.authorized(request) {
		response.Header().Set("WWW-Authenticate", "Bearer")
		writeOTLPStatus(response, http.StatusUnauthorized, mediaType, codes.Unauthenticated, "missing or invalid bearer token")
		return
	}
	body, err := readBody(response, request, receiver.config.MaxRequestBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			writeOTLPStatus(response, http.StatusRequestEntityTooLarge, mediaType, codes.ResourceExhausted, err.Error())
			return
		}
		writeOTLPStatus(response, http.StatusBadRequest, mediaType, codes.InvalidArgument, err.Error())
		return
	}
	var observations []inputpkg.Observation
	var source string
	switch mediaType {
	case mediaTypeJSON:
		observations, source, err = inputpkg.ParseOTLP(body, receiver.config.Source)
	case mediaTypeProtobuf:
		var exportRequest collectortracepb.ExportTraceServiceRequest
		if decodeErr := (proto.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, &exportRequest); decodeErr != nil {
			err = fmt.Errorf("decode OTLP protobuf: %w", decodeErr)
		} else {
			observations, source, err = inputpkg.ParseOTLPProto(&exportRequest, receiver.config.Source)
		}
	}
	if err != nil {
		receiver.recordFailure()
		writeOTLPStatus(response, http.StatusBadRequest, mediaType, codes.InvalidArgument, err.Error())
		return
	}
	if err := receiver.accept(observations, source); err != nil {
		receiver.recordFailure()
		writeOTLPStatus(response, http.StatusInternalServerError, mediaType, codes.Internal, "persist evidence snapshot: "+err.Error())
		return
	}
	if mediaType == mediaTypeProtobuf {
		writeProto(response, http.StatusOK, &collectortracepb.ExportTraceServiceResponse{})
		return
	}
	writeJSON(response, http.StatusOK, struct{}{})
}

// Export implements the OTLP/gRPC TraceService.
func (receiver *Receiver) Export(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	if !receiver.authorizedGRPC(ctx) {
		return nil, grpcstatus.Error(codes.Unauthenticated, "missing or invalid bearer token")
	}
	observations, source, err := inputpkg.ParseOTLPProto(request, receiver.config.Source)
	if err != nil {
		receiver.recordFailure()
		return nil, grpcstatus.Error(codes.InvalidArgument, err.Error())
	}
	if err := receiver.accept(observations, source); err != nil {
		receiver.recordFailure()
		return nil, grpcstatus.Error(codes.Internal, "persist evidence snapshot: "+err.Error())
	}
	return &collectortracepb.ExportTraceServiceResponse{}, nil
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
	return receiver.authorizedHeader(request.Header.Get("Authorization"))
}

func (receiver *Receiver) authorizedGRPC(ctx context.Context) bool {
	if receiver.config.AuthToken == "" {
		return true
	}
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	for _, value := range values {
		if receiver.authorizedHeader(value) {
			return true
		}
	}
	return false
}

func (receiver *Receiver) authorizedHeader(header string) bool {
	if receiver.config.AuthToken == "" {
		return true
	}
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
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
	trusted := receiver.config.SourceTrust.Apply(unique)
	unique = trusted.Observations
	receiver.stats.EvidenceDowngrades += uint64(trusted.Downgraded)

	now := receiver.config.Now().UTC()
	if len(unique) > 0 {
		ready := receiver.resolveTraceContexts(unique)
		receiver.trimTraceContexts()
		batch := normalize.BuildWithOptions(ready, source, now, normalize.Options{
			SensitiveHMACKey: receiver.config.SensitiveHMACKey,
		})
		if len(ready) > 0 {
			receiver.graph = aggregate.Merge(receiver.graph, batch, now)
		}
		receiver.stats.AcceptedSpans += uint64(len(unique))
		receiver.stats.LastAcceptedAt = now
	}
	receiver.stats.PendingSpans = uint64(len(receiver.pending))
	if receiver.graph.GeneratedAt.IsZero() {
		receiver.graph.GeneratedAt = now
	}
	return receiver.persist()
}

// resolveTraceContexts keeps only a metadata allowlist while waiting for a
// parent span that may arrive in a later OTLP export request. This prevents a
// child LLM/tool span from being assigned to service.name before its explicit
// agent identity is available.
func (receiver *Receiver) resolveTraceContexts(observations []inputpkg.Observation) []inputpkg.Observation {
	work := append([]inputpkg.Observation(nil), receiver.pending...)
	for _, observation := range observations {
		work = append(work, normalize.MetadataOnlyObservation(observation, receiver.config.SensitiveHMACKey))
	}
	type resolvedObservation struct {
		index   int
		context normalize.AgentContext
	}
	waitingByParent := make(map[string][]int)
	queue := make([]resolvedObservation, 0, len(work))
	resolved := make([]bool, len(work))
	for index, observation := range work {
		context := normalize.AgentContextFromAttributes(observation.Attributes)
		if !context.Empty() {
			queue = append(queue, resolvedObservation{index: index, context: context})
			continue
		}
		if observation.ParentSpanID == "" || observation.TraceID == "" {
			queue = append(queue, resolvedObservation{index: index})
			continue
		}
		parentKey := strings.ToLower(observation.TraceID) + "\x00" + strings.ToLower(observation.ParentSpanID)
		if parentContext, known := receiver.contexts[parentKey]; known {
			queue = append(queue, resolvedObservation{index: index, context: parentContext})
			continue
		}
		waitingByParent[parentKey] = append(waitingByParent[parentKey], index)
	}

	ready := make([]inputpkg.Observation, 0, len(work))
	for queueIndex := 0; queueIndex < len(queue); queueIndex++ {
		item := queue[queueIndex]
		if resolved[item.index] {
			continue
		}
		observation := work[item.index]
		if !item.context.Empty() {
			observation = normalize.ApplyAgentContext(observation, item.context)
		}
		work[item.index] = observation
		resolved[item.index] = true
		ready = append(ready, observation)
		spanKey := observationKey(observation)
		if spanKey == "" {
			continue
		}
		receiver.contexts[spanKey] = item.context
		for _, childIndex := range waitingByParent[spanKey] {
			queue = append(queue, resolvedObservation{index: childIndex, context: item.context})
		}
		delete(waitingByParent, spanKey)
	}

	unresolved := make([]inputpkg.Observation, 0, len(work)-len(ready))
	for index, observation := range work {
		if !resolved[index] {
			unresolved = append(unresolved, observation)
		}
	}

	if overflow := len(unresolved) - receiver.config.MaxDedupeItems; overflow > 0 {
		for _, observation := range unresolved[:overflow] {
			if key := observationKey(observation); key != "" {
				receiver.contexts[key] = normalize.AgentContext{}
			}
		}
		ready = append(ready, unresolved[:overflow]...)
		unresolved = unresolved[overflow:]
	}
	receiver.pending = unresolved
	return ready
}

func (receiver *Receiver) trimTraceContexts() {
	for key := range receiver.contexts {
		if _, retained := receiver.seen[key]; !retained {
			delete(receiver.contexts, key)
		}
	}
}

func (receiver *Receiver) trimDedupe() {
	overflow := len(receiver.seenOrder) - receiver.config.MaxDedupeItems
	if overflow <= 0 {
		return
	}
	for _, key := range receiver.seenOrder[:overflow] {
		delete(receiver.seen, key)
		delete(receiver.contexts, key)
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
	receiver.config.SourceTrust.CapGraphInPlace(&receiver.graph)
	receiver.graph.SchemaVersion = model.SchemaVersion
	if receiver.graph.Metadata == nil {
		receiver.graph.Metadata = make(map[string]string)
	}
	receiver.graph.Metadata["normalizer.version"] = model.SchemaVersion
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

func writeOTLPStatus(response http.ResponseWriter, httpStatus int, mediaType string, code codes.Code, message string) {
	statusMessage := grpcstatus.New(code, message).Proto()
	if mediaType == mediaTypeProtobuf {
		writeProto(response, httpStatus, statusMessage)
		return
	}
	data, err := protojson.Marshal(statusMessage)
	if err != nil {
		writeStatus(response, http.StatusInternalServerError, "encode OTLP status")
		return
	}
	response.Header().Set("Content-Type", mediaTypeJSON)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(httpStatus)
	_, _ = response.Write(append(data, '\n'))
}

func writeProto(response http.ResponseWriter, status int, message proto.Message) {
	data, err := proto.Marshal(message)
	if err != nil {
		writeStatus(response, http.StatusInternalServerError, "encode protobuf response")
		return
	}
	response.Header().Set("Content-Type", mediaTypeProtobuf)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(status)
	_, _ = response.Write(data)
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
