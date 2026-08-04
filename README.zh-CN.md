# AI Evidence BOM

AI Evidence BOM 是一个早期、厂商中立的验证项目：它把生成式 AI 和 Agent 的运行时遥测转换成保护隐私的证据图，并可导出 CycloneDX AI/ML BOM。

它关注的不是“代码中声明了什么”，而是：

> 实际运行时观察到了哪些 Agent、模型、工具、MCP Server、Prompt 和数据源，它们后来发生了什么变化？

当前为实验性 v0.6，不是合规认证工具、恶意软件判定工具，也无法发现未进行插桩的全部 AI 组件。

## 当前能力

- 读取 OTLP JSON 和简化的 observation JSON；
- 通过 `/v1/traces` 接收 OTLP/HTTP JSON 或 Protobuf，并通过 4317 端口接收 OTLP/gRPC；
- 构建厂商中立的 Agent 证据关系图；
- 跨 OTLP 批次使用父子 span 关系，把模型和工具归属到正确的 Agent，并消除框架汇总 span 造成的重复模型；
- 提供 Dify 与 Microsoft Agent Framework 的源码契约和可执行兼容性检查；
- 区分 `inferred`、`declared`、`observed`、`verified` 四级证据；
- 导出 CycloneDX 1.7；
- 检测模型、工具、MCP、数据源和权限变化；
- 使用 JSON 策略作为 CI 门禁；
- 使用 Ed25519 签名并验证证据文件；
- 默认只处理元数据，不保存 Prompt、响应、工具参数或工具结果。
- 对在线请求限制大小，支持 gzip、可选 Bearer Token，并去除近期重复 span。

## 快速体验

要求 Go 1.26.5 或更高版本；更早的 Go 1.26 补丁版本包含已在 1.26.5 修复的标准库漏洞。

```bash
go install github.com/Aaron911/ai-evidence-bom/cmd/aiebom@latest
```

或者在源码目录构建：

```bash
go build -o ./bin/aiebom ./cmd/aiebom

./bin/aiebom scan \
  --input examples/otlp-before.json \
  --graph-out work/before.evidence.json \
  --bom-out work/before.cdx.json

./bin/aiebom scan \
  --input examples/otlp-after.json \
  --graph-out work/after.evidence.json \
  --bom-out work/after.cdx.json

./bin/aiebom diff \
  --before work/before.evidence.json \
  --after work/after.evidence.json \
  --output work/diff.json

./bin/aiebom policy \
  --input work/after.evidence.json \
  --policy examples/policy.json \
  --output work/policy-report.json
```

示例策略会故意拒绝新出现的 `shell.execute` 能力，并以状态码 3 退出。

## 在线采集

启动仅监听本机的双协议接收器：

```bash
./bin/aiebom collect \
  --listen 127.0.0.1:4318 \
  --grpc-listen 127.0.0.1:4317 \
  --graph-out work/live.evidence.json \
  --bom-out work/live.cdx.json
```

发送示例 OTLP JSON：

```bash
curl --fail-with-body \
  -H 'Content-Type: application/json' \
  --data-binary @examples/otlp-before.json \
  http://127.0.0.1:4318/v1/traces
```

同一个 HTTP 地址还接受 `application/x-protobuf`，gRPC 端口实现标准 OTLP `TraceService/Export`。可通过 `GET /healthz` 检查健康状态，并通过 `/v1/evidence`、`/v1/bom`、`/v1/stats` 查看实时结果。

现在可以直接接在 OpenTelemetry Collector 后面：可使用 [OTLP/HTTP Protobuf 配置](examples/otel-collector-http.yaml) 或 [OTLP/gRPC 配置](examples/otel-collector-grpc.yaml)。鉴权、请求限制、TLS 边界和协议范围见英文 [运行时接收器文档](docs/RUNTIME_RECEIVER.md)。v0.6 只处理 traces，不接收 metrics、logs 或 profiles。

无需模型 API Key 即可运行三项确定性兼容性检查：

```bash
scripts/live/verify_agent_framework.sh
scripts/live/verify_dify_instrumentation.sh
scripts/live/verify_dify_runtime.sh
```

第一项运行 Microsoft Agent Framework 的真实 Agent、模型遥测和工具调用链路；第二项快速隔离执行 Dify 1.16.1 的真实 OTel handler/parser；第三项启动官方最小 Dify 应用栈，安装经过 SHA-256 校验的官方 OpenAI 插件包，并在无模型 API Key、无付费调用的条件下执行包含 LLM 和工具节点的工作流。第三项需要 Docker，冷启动需要联网下载固定版本源码和插件。证据等级和限制见 [兼容矩阵](docs/COMPATIBILITY.md) 与 [v0.6 验证记录](docs/evidence/v0.6.0.md)。

兼容矩阵见 [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md)，每轮方向校准记录见 [docs/DIRECTION.md](docs/DIRECTION.md)。完整使用方法、架构和项目边界请查看英文 [README.md](README.md)。
