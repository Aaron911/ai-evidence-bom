package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	serverName       = "demo-security-tools"
	serverVersion    = "1.0.0"
	agentID          = "security-agent"
	expectedProtocol = "2026-07-28"
	sensitiveSchema  = "PRIVATE_MCP_SCHEMA_MUST_NOT_LEAK"
	sensitiveArg     = "PRIVATE_MCP_ARGUMENT_MUST_NOT_LEAK"
	sensitiveResult  = "PRIVATE_MCP_RESULT_MUST_NOT_LEAK"
)

type serverIdentity struct {
	mu              sync.RWMutex
	name            string
	version         string
	protocolVersion string
	artifactURI     string
	artifactSHA256  string
}

func (identity *serverIdentity) set(name, version, protocolVersion string) {
	identity.mu.Lock()
	defer identity.mu.Unlock()
	identity.name = name
	identity.version = version
	identity.protocolVersion = protocolVersion
}

func (identity *serverIdentity) setArtifact(uri, digest string) {
	identity.mu.Lock()
	defer identity.mu.Unlock()
	identity.artifactURI = uri
	identity.artifactSHA256 = digest
}

func (identity *serverIdentity) attributes() []attribute.KeyValue {
	identity.mu.RLock()
	defer identity.mu.RUnlock()
	values := []attribute.KeyValue{attribute.String("network.transport", "pipe")}
	if identity.protocolVersion != "" {
		values = append(values, attribute.String("mcp.protocol.version", identity.protocolVersion))
	}
	if identity.name != "" {
		values = append(values,
			attribute.String("aiebom.mcp.server.name", identity.name),
			attribute.String("aiebom.mcp.server.version", identity.version),
			attribute.String("aiebom.mcp.server.identity_source", "server_info"),
		)
	}
	if identity.artifactURI != "" {
		values = append(values,
			attribute.String("aiebom.artifact.uri", identity.artifactURI),
			attribute.String("aiebom.artifact.sha256", identity.artifactSHA256),
		)
	}
	return values
}

func main() {
	role := flag.String("role", "client", "client or server")
	variant := flag.String("variant", "before", "before or after capability set")
	otlpEndpoint := flag.String("otlp-endpoint", "", "OTLP/HTTP trace endpoint")
	artifactURI := flag.String("artifact-uri", "", "operator-supplied stable source artifact URI")
	artifactSHA256 := flag.String("artifact-sha256", "", "operator-supplied source artifact SHA-256")
	flag.Parse()

	if *variant != "before" && *variant != "after" && *variant != "vulnerable" {
		log.Fatalf("unsupported variant %q", *variant)
	}
	var err error
	switch *role {
	case "server":
		err = runServer(*variant)
	case "client":
		if *otlpEndpoint == "" {
			log.Fatal("--otlp-endpoint is required for client role")
		}
		if (*artifactURI == "") != (*artifactSHA256 == "") {
			log.Fatal("--artifact-uri and --artifact-sha256 must be supplied together")
		}
		err = runClient(*variant, *otlpEndpoint, *artifactURI, *artifactSHA256)
	default:
		log.Fatalf("unsupported role %q", *role)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

type weatherInput struct {
	Location string `json:"location" jsonschema:"location identifier; PRIVATE_MCP_SCHEMA_MUST_NOT_LEAK"`
}

type shellInput struct {
	Command string `json:"command" jsonschema:"command text; PRIVATE_MCP_SCHEMA_MUST_NOT_LEAK"`
}

func runServer(variant string) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{},
		Logger:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	weatherDestructive := false
	mcp.AddTool(server, &mcp.Tool{
		Name:        "weather.lookup",
		Description: "deterministic weather lookup; " + sensitiveSchema,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &weatherDestructive},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input weatherInput) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: "weather for " + input.Location + ": " + sensitiveResult},
		}}, nil, nil
	})
	if variant == "after" || variant == "vulnerable" {
		shellDestructive := true
		mcp.AddTool(server, &mcp.Tool{
			Name:        "shell.execute",
			Description: "deterministic shell fixture; " + sensitiveSchema,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &shellDestructive},
		}, func(_ context.Context, _ *mcp.CallToolRequest, input shellInput) (*mcp.CallToolResult, any, error) {
			if variant == "vulnerable" {
				// Deliberately vulnerable static-analysis fixture. The live check
				// discovers but never invokes this tool.
				output, err := exec.Command("sh", "-c", input.Command).CombinedOutput()
				if err != nil {
					return nil, nil, err
				}
				return &mcp.CallToolResult{Content: []mcp.Content{
					&mcp.TextContent{Text: string(output)},
				}}, nil, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{
				&mcp.TextContent{Text: "not executed: " + input.Command + ": " + sensitiveResult},
			}}, nil, nil
		})
	}
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func runClient(variant, endpoint, artifactURI, artifactSHA256 string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return fmt.Errorf("create OTLP exporter: %w", err)
	}
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", "mcp-runtime-client"),
		attribute.String("service.version", "0.7.0"),
	))
	if err != nil {
		return fmt.Errorf("create OTel resource: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	defer func() {
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if shutdownErr := provider.Shutdown(shutdownContext); shutdownErr != nil {
			log.Printf("shut down tracer provider: %v", shutdownErr)
		}
	}()
	otel.SetTracerProvider(provider)
	tracer := provider.Tracer("github.com/Aaron911/ai-evidence-bom/mcp-runtime", trace.WithInstrumentationVersion("0.7.0"))
	identity := new(serverIdentity)
	identity.setArtifact(artifactURI, artifactSHA256)

	agentContext, agentSpan := tracer.Start(ctx, "invoke_agent "+agentID,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "invoke_agent"),
			attribute.String("gen_ai.agent.id", agentID),
			attribute.String("gen_ai.agent.name", agentID),
			attribute.String("gen_ai.agent.version", "1.0.0"),
		),
	)
	defer agentSpan.End()

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate live-check executable: %w", err)
	}
	client, session, err := connectMCP(agentContext, executable, variant)
	if err != nil {
		return fmt.Errorf("connect to MCP server: %w", err)
	}
	defer session.Close()

	initialized := session.InitializeResult()
	if initialized == nil || initialized.ServerInfo == nil {
		return errors.New("MCP discovery did not return serverInfo")
	}
	if initialized.ServerInfo.Name != serverName || initialized.ServerInfo.Version != serverVersion {
		return fmt.Errorf("unexpected serverInfo: %+v", initialized.ServerInfo)
	}
	if initialized.ProtocolVersion != expectedProtocol {
		return fmt.Errorf("protocol version=%q want=%q", initialized.ProtocolVersion, expectedProtocol)
	}
	identity.set(initialized.ServerInfo.Name, initialized.ServerInfo.Version, initialized.ProtocolVersion)
	// The upstream SDK has no built-in OTel instrumentation. Install the
	// application middleware only after protocol discovery so discovery remains
	// the identity source and subsequent list/call RPCs carry that identity.
	client.AddSendingMiddleware(telemetryMiddleware(tracer, identity))

	listed, err := session.ListTools(agentContext, nil)
	if err != nil {
		return fmt.Errorf("list MCP tools: %w", err)
	}
	if err := validateToolSet(variant, listed.Tools); err != nil {
		return err
	}
	if err := emitDeclaredCapabilities(tracer, identity, listed.Tools); err != nil {
		return err
	}

	result, err := session.CallTool(agentContext, &mcp.CallToolParams{
		Name:      "weather.lookup",
		Arguments: map[string]any{"location": sensitiveArg},
	})
	if err != nil {
		return fmt.Errorf("call MCP tool: %w", err)
	}
	if !resultContains(result, sensitiveArg) || !resultContains(result, sensitiveResult) {
		return errors.New("MCP tool result did not exercise sensitive argument and result markers")
	}

	agentSpan.End()
	if err := provider.ForceFlush(ctx); err != nil {
		return fmt.Errorf("flush OTLP spans: %w", err)
	}
	return nil
}

func connectMCP(ctx context.Context, executable, variant string) (*mcp.Client, *mcp.ClientSession, error) {
	// Retry only transient stdio discovery closure; semantic/protocol failures
	// return immediately and the total attempt count remains bounded. A failed
	// connect closes its Client, so every retry must use a fresh instance.
	var lastError error
	for attempt := 1; attempt <= 3; attempt++ {
		client := mcp.NewClient(&mcp.Implementation{Name: "aiebom-live-client", Version: "0.7.0"}, &mcp.ClientOptions{
			Capabilities: &mcp.ClientCapabilities{},
			Logger:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		})
		command := exec.Command(executable, "--role=server", "--variant="+variant)
		command.Stderr = os.Stderr
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
		if err == nil {
			return client, session, nil
		}
		lastError = err
		if !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "connection closed") {
			break
		}
		log.Printf("transient MCP stdio connect attempt %d failed: %v", attempt, err)
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
	return nil, nil, lastError
}

func telemetryMiddleware(tracer trace.Tracer, identity *serverIdentity) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			attributes := append([]attribute.KeyValue{attribute.String("mcp.method.name", method)}, identity.attributes()...)
			spanName := method
			if session, ok := request.GetSession().(*mcp.ClientSession); ok {
				if initialized := session.InitializeResult(); initialized != nil && initialized.ProtocolVersion != "" {
					attributes = append(attributes, attribute.String("mcp.protocol.version", initialized.ProtocolVersion))
				}
			}
			if params, ok := request.GetParams().(*mcp.CallToolParams); ok {
				spanName += " " + params.Name
				attributes = append(attributes,
					attribute.String("gen_ai.operation.name", "execute_tool"),
					attribute.String("gen_ai.tool.name", params.Name),
				)
			}
			spanContext, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(attributes...),
			)
			defer span.End()
			result, err := next(spanContext, method, request)
			if err != nil {
				log.Printf("MCP method %s failed: %v", method, err)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return result, err
		}
	}
}

func emitDeclaredCapabilities(tracer trace.Tracer, identity *serverIdentity, tools []*mcp.Tool) error {
	for _, tool := range tools {
		encodedSchema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return fmt.Errorf("encode input schema for tool %q: %w", tool.Name, err)
		}
		digest := sha256.Sum256(encodedSchema)
		attributes := append(identity.attributes(),
			attribute.String("aiebom.evidence.level", "declared"),
			attribute.String("aiebom.mcp.discovery.source", "tools/list"),
			attribute.String("aiebom.mcp.tool.input_schema_sha256", hex.EncodeToString(digest[:])),
			attribute.String("mcp.method.name", "tools/list"),
			attribute.String("gen_ai.tool.name", tool.Name),
		)
		if tool.Annotations != nil {
			attributes = append(attributes,
				attribute.Bool("aiebom.mcp.tool.read_only", tool.Annotations.ReadOnlyHint),
				attribute.Bool("aiebom.mcp.tool.annotations_untrusted", true),
			)
			if tool.Annotations.DestructiveHint != nil {
				attributes = append(attributes, attribute.Bool("aiebom.mcp.tool.destructive", *tool.Annotations.DestructiveHint))
			}
		}
		_, span := tracer.Start(context.Background(), "mcp.capability "+tool.Name,
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(attributes...),
		)
		span.End()
	}
	return nil
}

func validateToolSet(variant string, tools []*mcp.Tool) error {
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["weather.lookup"] {
		return errors.New("weather.lookup missing from tools/list")
	}
	if variant == "before" && names["shell.execute"] {
		return errors.New("before variant unexpectedly declared shell.execute")
	}
	if (variant == "after" || variant == "vulnerable") && !names["shell.execute"] {
		return fmt.Errorf("%s variant did not declare shell.execute", variant)
	}
	return nil
}

func resultContains(result *mcp.CallToolResult, marker string) bool {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && strings.Contains(text.Text, marker) {
			return true
		}
	}
	return false
}
