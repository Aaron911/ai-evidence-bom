package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"

	"github.com/Aaron911/ai-evidence-bom/internal/collector"
	"github.com/Aaron911/ai-evidence-bom/internal/cyclonedx"
	"github.com/Aaron911/ai-evidence-bom/internal/graphdiff"
	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
	"github.com/Aaron911/ai-evidence-bom/internal/normalize"
	"github.com/Aaron911/ai-evidence-bom/internal/policy"
	"github.com/Aaron911/ai-evidence-bom/internal/signing"
)

const usageText = `AI Evidence BOM

Usage:
  aiebom scan    --input traces.json --graph-out evidence.json [--bom-out bom.cdx.json]
  aiebom collect --graph-out evidence.json [--listen 127.0.0.1:4318] [--grpc-listen 127.0.0.1:4317]
  aiebom export  --input evidence.json --output bom.cdx.json
  aiebom diff    --before old.json --after new.json --output diff.json [--fail-on-change]
  aiebom policy  --input evidence.json --policy policy.json --output report.json
  aiebom keygen  --private-key private.pem --public-key public.pem
  aiebom sign    --input evidence.json --private-key private.pem --output evidence.sig.json
  aiebom verify  --input evidence.json --public-key public.pem --signature evidence.sig.json

Inputs to scan may be compact observation JSON or OTLP JSON with resourceSpans.
Collect accepts OTLP/HTTP JSON or protobuf at POST /v1/traces and OTLP/gRPC TraceService exports.
Prompt and tool-call content is not retained; optional prompt fingerprints use keyed HMAC-SHA-256.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "scan":
		err = runScan(os.Args[2:])
	case "collect":
		err = runCollect(os.Args[2:])
	case "export":
		err = runExport(os.Args[2:])
	case "diff":
		err = runDiff(os.Args[2:])
	case "policy":
		err = runPolicy(os.Args[2:])
	case "keygen":
		err = runKeygen(os.Args[2:])
	case "sign":
		err = runSign(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "version", "--version", "-version":
		fmt.Println(model.SchemaVersion)
		return
	case "help", "--help", "-h":
		fmt.Print(usageText)
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		var exitErr exitError
		if errors.As(err, &exitErr) {
			fmt.Fprintln(os.Stderr, exitErr.Error())
			os.Exit(exitErr.code)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runCollect(args []string) error {
	flags := flag.NewFlagSet("collect", flag.ContinueOnError)
	listenAddress := flags.String("listen", "127.0.0.1:4318", "OTLP/HTTP listen address")
	grpcListenAddress := flags.String("grpc-listen", "127.0.0.1:4317", "OTLP/gRPC listen address; empty disables gRPC")
	graphOut := flags.String("graph-out", "", "continuously updated evidence graph JSON")
	bomOut := flags.String("bom-out", "", "optional continuously updated CycloneDX JSON")
	source := flags.String("source", "otlp", "receiver source name")
	authTokenPath := flags.String("auth-token-file", "", "optional bearer token file")
	hmacKeyPath := flags.String("sensitive-hmac-key-file", "", "optional key for privacy-preserving prompt fingerprints")
	maxRequestBytes := flags.Int64("max-request-bytes", collector.DefaultMaxRequestBytes, "maximum request bytes before and after decompression")
	maxDedupeItems := flags.Int("max-dedupe-items", collector.DefaultMaxDedupeItems, "recent span IDs retained for retry deduplication")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *graphOut == "" {
		return fmt.Errorf("collect requires --graph-out")
	}
	if *listenAddress == "" && *grpcListenAddress == "" {
		return fmt.Errorf("collect requires at least one of --listen or --grpc-listen")
	}
	if *maxRequestBytes > int64(^uint(0)>>1) {
		return fmt.Errorf("max request bytes exceeds platform limit")
	}

	authToken := ""
	if *authTokenPath != "" {
		data, err := os.ReadFile(*authTokenPath)
		if err != nil {
			return fmt.Errorf("read auth token: %w", err)
		}
		authToken = strings.TrimSpace(string(data))
		if authToken == "" {
			return fmt.Errorf("auth token file must not be empty")
		}
	}
	if ((*listenAddress != "" && !isLoopbackListen(*listenAddress)) || (*grpcListenAddress != "" && !isLoopbackListen(*grpcListenAddress))) && authToken == "" {
		return fmt.Errorf("non-loopback listen address requires --auth-token-file")
	}

	var sensitiveHMACKey []byte
	if *hmacKeyPath != "" {
		var err error
		sensitiveHMACKey, err = os.ReadFile(*hmacKeyPath)
		if err != nil {
			return fmt.Errorf("read sensitive HMAC key: %w", err)
		}
		if len(sensitiveHMACKey) < 32 {
			return fmt.Errorf("sensitive HMAC key must contain at least 32 bytes")
		}
	}

	receiver, err := collector.New(collector.Config{
		GraphOut:         *graphOut,
		BOMOut:           *bomOut,
		Source:           *source,
		AuthToken:        authToken,
		SensitiveHMACKey: sensitiveHMACKey,
		MaxRequestBytes:  *maxRequestBytes,
		MaxDedupeItems:   *maxDedupeItems,
	})
	if err != nil {
		return err
	}
	var httpServer *http.Server
	var httpListener net.Listener
	if *listenAddress != "" {
		httpListener, err = net.Listen("tcp", *listenAddress)
		if err != nil {
			return fmt.Errorf("listen for OTLP/HTTP: %w", err)
		}
		httpServer = &http.Server{
			Handler:           receiver,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       2 * time.Minute,
			MaxHeaderBytes:    1 << 20,
		}
	}

	var grpcServer *grpc.Server
	var grpcListener net.Listener
	if *grpcListenAddress != "" {
		grpcListener, err = net.Listen("tcp", *grpcListenAddress)
		if err != nil {
			if httpListener != nil {
				_ = httpListener.Close()
			}
			return fmt.Errorf("listen for OTLP/gRPC: %w", err)
		}
		grpcServer = grpc.NewServer(grpc.MaxRecvMsgSize(int(*maxRequestBytes)))
		collectortracepb.RegisterTraceServiceServer(grpcServer, receiver)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errorsChannel := make(chan error, 2)
	if httpServer != nil {
		go func() {
			fmt.Fprintf(os.Stderr, "listening for OTLP/HTTP JSON and protobuf traces on http://%s/v1/traces\n", httpListener.Addr())
			err := httpServer.Serve(httpListener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errorsChannel <- fmt.Errorf("serve OTLP/HTTP: %w", err)
			}
		}()
	}
	if grpcServer != nil {
		go func() {
			fmt.Fprintf(os.Stderr, "listening for OTLP/gRPC traces on %s\n", grpcListener.Addr())
			if err := grpcServer.Serve(grpcListener); err != nil {
				errorsChannel <- fmt.Errorf("serve OTLP/gRPC: %w", err)
			}
		}()
	}

	select {
	case err := <-errorsChannel:
		if httpServer != nil {
			_ = httpServer.Close()
		}
		if grpcServer != nil {
			grpcServer.Stop()
		}
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if httpServer != nil {
			if err := httpServer.Shutdown(shutdownContext); err != nil {
				if grpcServer != nil {
					grpcServer.Stop()
				}
				return fmt.Errorf("shutdown OTLP/HTTP receiver: %w", err)
			}
		}
		if grpcServer != nil {
			stopped := make(chan struct{})
			go func() {
				grpcServer.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
			case <-shutdownContext.Done():
				grpcServer.Stop()
				return fmt.Errorf("shutdown OTLP/gRPC receiver: %w", shutdownContext.Err())
			}
		}
		fmt.Fprintln(os.Stderr, "collector stopped")
		return nil
	}
}

func isLoopbackListen(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type exitError struct {
	code    int
	message string
}

func (e exitError) Error() string { return e.message }

func runScan(args []string) error {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	inputPath := flags.String("input", "", "compact observation or OTLP JSON")
	graphOut := flags.String("graph-out", "", "evidence graph JSON")
	bomOut := flags.String("bom-out", "", "optional CycloneDX JSON")
	source := flags.String("source", "", "fallback source name")
	hmacKeyPath := flags.String("sensitive-hmac-key-file", "", "optional key for privacy-preserving prompt fingerprints")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" || *graphOut == "" {
		return fmt.Errorf("scan requires --input and --graph-out")
	}
	data, err := readFileOrStdin(*inputPath)
	if err != nil {
		return err
	}
	observations, detectedSource, err := inputpkg.Parse(data, *source)
	if err != nil {
		return err
	}
	var options normalize.Options
	if *hmacKeyPath != "" {
		options.SensitiveHMACKey, err = os.ReadFile(*hmacKeyPath)
		if err != nil {
			return fmt.Errorf("read sensitive HMAC key: %w", err)
		}
		if len(options.SensitiveHMACKey) < 32 {
			return fmt.Errorf("sensitive HMAC key must contain at least 32 bytes")
		}
	}
	graph := normalize.BuildWithOptions(observations, detectedSource, time.Now().UTC(), options)
	if err := writeJSON(*graphOut, graph, 0o644); err != nil {
		return err
	}
	if *bomOut != "" {
		if err := writeJSON(*bomOut, cyclonedx.Export(graph), 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "generated %d nodes and %d relationships\n", len(graph.Nodes), len(graph.Edges))
	return nil
}

func runExport(args []string) error {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	inputPath := flags.String("input", "", "evidence graph JSON")
	outputPath := flags.String("output", "", "CycloneDX JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" || *outputPath == "" {
		return fmt.Errorf("export requires --input and --output")
	}
	var graph model.Graph
	if err := readJSON(*inputPath, &graph); err != nil {
		return err
	}
	return writeJSON(*outputPath, cyclonedx.Export(graph), 0o644)
}

func runDiff(args []string) error {
	flags := flag.NewFlagSet("diff", flag.ContinueOnError)
	beforePath := flags.String("before", "", "previous evidence graph")
	afterPath := flags.String("after", "", "current evidence graph")
	outputPath := flags.String("output", "-", "diff JSON")
	failOnChange := flags.Bool("fail-on-change", false, "exit 2 when changes exist")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *beforePath == "" || *afterPath == "" {
		return fmt.Errorf("diff requires --before and --after")
	}
	var before, after model.Graph
	if err := readJSON(*beforePath, &before); err != nil {
		return err
	}
	if err := readJSON(*afterPath, &after); err != nil {
		return err
	}
	result := graphdiff.Compare(before, after, time.Now().UTC())
	if err := writeJSON(*outputPath, result, 0o644); err != nil {
		return err
	}
	if *failOnChange && result.HasChanges() {
		return exitError{code: 2, message: "evidence graph changed"}
	}
	return nil
}

func runPolicy(args []string) error {
	flags := flag.NewFlagSet("policy", flag.ContinueOnError)
	inputPath := flags.String("input", "", "evidence graph JSON")
	policyPath := flags.String("policy", "", "policy JSON")
	outputPath := flags.String("output", "-", "policy report JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" || *policyPath == "" {
		return fmt.Errorf("policy requires --input and --policy")
	}
	var graph model.Graph
	var rules policy.Policy
	if err := readJSON(*inputPath, &graph); err != nil {
		return err
	}
	if err := readJSON(*policyPath, &rules); err != nil {
		return err
	}
	report, err := policy.Evaluate(graph, rules, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := writeJSON(*outputPath, report, 0o644); err != nil {
		return err
	}
	if !report.Passed {
		return exitError{code: 3, message: fmt.Sprintf("policy failed with %d violation(s)", len(report.Violations))}
	}
	return nil
}

func runKeygen(args []string) error {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	privatePath := flags.String("private-key", "", "private key PEM")
	publicPath := flags.String("public-key", "", "public key PEM")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *privatePath == "" || *publicPath == "" {
		return fmt.Errorf("keygen requires --private-key and --public-key")
	}
	privatePEM, publicPEM, err := signing.GenerateKeyPair()
	if err != nil {
		return err
	}
	if err := writeExclusive(*privatePath, privatePEM, 0o600); err != nil {
		return err
	}
	if err := writeExclusive(*publicPath, publicPEM, 0o644); err != nil {
		return err
	}
	return nil
}

func runSign(args []string) error {
	flags := flag.NewFlagSet("sign", flag.ContinueOnError)
	inputPath := flags.String("input", "", "file to sign")
	privatePath := flags.String("private-key", "", "private key PEM")
	outputPath := flags.String("output", "", "signature envelope JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" || *privatePath == "" || *outputPath == "" {
		return fmt.Errorf("sign requires --input, --private-key and --output")
	}
	payload, err := os.ReadFile(*inputPath)
	if err != nil {
		return fmt.Errorf("read payload: %w", err)
	}
	privatePEM, err := os.ReadFile(*privatePath)
	if err != nil {
		return fmt.Errorf("read private key: %w", err)
	}
	envelope, err := signing.Sign(payload, privatePEM, time.Now().UTC())
	if err != nil {
		return err
	}
	return writeJSON(*outputPath, envelope, 0o644)
}

func runVerify(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	inputPath := flags.String("input", "", "signed file")
	publicPath := flags.String("public-key", "", "public key PEM")
	signaturePath := flags.String("signature", "", "signature envelope JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" || *publicPath == "" || *signaturePath == "" {
		return fmt.Errorf("verify requires --input, --public-key and --signature")
	}
	payload, err := os.ReadFile(*inputPath)
	if err != nil {
		return fmt.Errorf("read payload: %w", err)
	}
	publicPEM, err := os.ReadFile(*publicPath)
	if err != nil {
		return fmt.Errorf("read public key: %w", err)
	}
	var envelope signing.Envelope
	if err := readJSON(*signaturePath, &envelope); err != nil {
		return err
	}
	if err := signing.Verify(payload, envelope, publicPEM); err != nil {
		return err
	}
	fmt.Println("signature valid")
	return nil
}

func readJSON(path string, destination any) error {
	data, err := readFileOrStdin(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func readFileOrStdin(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		return data, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

func writeJSON(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	data = append(data, '\n')
	if path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return fmt.Errorf("create key directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
