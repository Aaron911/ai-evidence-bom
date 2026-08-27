# AI Evidence BOM

AI Evidence BOM 是一个早期、厂商中立的验证项目：它把生成式 AI 和 Agent 的运行时遥测转换成保护隐私的证据图，并可导出 CycloneDX AI/ML BOM。

它关注的不是“代码中声明了什么”，而是：

> 实际运行时观察到了哪些 Agent、模型、工具、MCP Server、Prompt 和数据源，它们后来发生了什么变化？

当前为实验性 v0.10，不是合规认证工具、恶意软件判定工具，也无法发现未进行插桩的全部 AI 组件。

## 当前能力

- 读取 OTLP JSON 和简化的 observation JSON；
- 通过 `/v1/traces` 接收 OTLP/HTTP JSON 或 Protobuf，并通过 4317 端口接收 OTLP/gRPC；
- 构建厂商中立的 Agent 证据关系图；
- 跨 OTLP 批次使用父子 span 关系，把模型和工具归属到正确的 Agent，并消除框架汇总 span 造成的重复模型；
- 提供 Dify 与 Microsoft Agent Framework 的源码契约和可执行兼容性检查；
- 结合 MCP 协议发现与运行时遥测，区分“服务端声明可用”与“Agent 实际调用”；
- 区分 `inferred`、`declared`、`observed`、`verified` 四级证据；
- 为每个来源设置证据等级上限：默认最高为 `observed`，只有运维策略精确授权的来源才能保留 `verified`，在线受保护来源还可绑定 OTLP 载荷之外的独立采集凭证；
- 对版本、摘要和保留属性记录字段级候选证据，确定性选择最强值并显式暴露冲突；
- 导出 CycloneDX 1.7，并在 CI 中使用校验和固定的官方 Schema 验证；
- 检测模型、工具、MCP、数据源和权限变化；
- 使用节点策略和有向图路径策略作为 CI 门禁；
- 使用 Ed25519 签名并验证精确文件字节，或签名采用 RFC 8785 规范化的证据图身份；
- 默认只处理元数据，不保存 Prompt、响应、工具参数或工具结果。
- 对在线请求限制大小，支持 gzip，区分全局读取凭证与仅可采集的来源凭证，并去除近期重复 span。

## 快速体验

要求 Go 1.26.6 或更高版本；更早的 Go 1.26 补丁版本包含已在 1.26.6 修复的可达标准库漏洞。

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

## 授权可信验证器

compact observation 不能再通过自报把自己升级为 `verified`。如果一个受控适配器确实完成了独立验证，需要由运维方精确授权其来源名称：

```bash
./bin/aiebom scan \
  --input examples/conflicting-model-evidence.json \
  --source-trust-policy examples/source-trust-policy.json \
  --graph-out work/conflict.evidence.json
```

规则按完整来源名称区分大小写匹配，不支持通配授权，也可以把某个来源进一步限制为 `declared` 或 `inferred`。

在线 `collect` 如果要给某个来源配置高于 `observed` 的权限，还必须通过 `--source-auth-policy` 把随机 Bearer Token 的 SHA-256 摘要绑定到该精确来源。Token 通过 OTLP HTTP/gRPC 都支持的 `Authorization` 请求头发送，不放进 OTLP 载荷；认证成功后，凭证绑定的来源会取代 `service.name` 作为证据权威来源。来源凭证只能写入遥测，不能读取图、BOM 或统计；同一来源可同时绑定新旧 Token 以便轮换。认证只证明“是配置的生产者发来的”，不会把普通 OTLP 自动升级为 `verified`，更不代表组件安全。详见 [在线来源认证](docs/RUNTIME_RECEIVER.md#bind-a-live-producer-to-an-evidence-source)、[v0.9 证据记录](docs/evidence/v0.9.0.md) 与 [v0.10 证据记录](docs/evidence/v0.10.0.md)。

## 可复现的证据签名

对证据图推荐使用规范签名模式：

```bash
./bin/aiebom keygen \
  --private-key work/evidence-private.pem \
  --public-key work/evidence-public.pem

./bin/aiebom sign \
  --input work/after.evidence.json \
  --private-key work/evidence-private.pem \
  --canonical-evidence \
  --output work/after.evidence.sig.json

./bin/aiebom verify \
  --input work/after.evidence.json \
  --public-key work/evidence-public.pem \
  --signature work/after.evidence.sig.json
```

该模式会严格解析证据图、拒绝重复或未知 JSON 字段、规范集合顺序和时区，再使用 [RFC 8785 JCS](https://www.rfc-editor.org/rfc/rfc8785.html) 生成签名身份。因此，缩进、对象字段顺序、节点/边顺序等传输格式变化不会改变摘要；任一被保留的证据字段变化都会导致验证失败。信封明确记录规范化模式，验证时不进行猜测。

不加 `--canonical-evidence` 时仍使用兼容旧版本的“精确字节签名”，可签名 BOM 或其他文件，但重新格式化会使签名失效。当前规范模式只支持本项目证据图，并不是 CycloneDX JSF/JWS 实现；信封中的 `createdAt` 只是未被签名的说明性元数据。

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

现在可以直接接在 OpenTelemetry Collector 后面：可使用 [OTLP/HTTP Protobuf 配置](examples/otel-collector-http.yaml) 或 [OTLP/gRPC 配置](examples/otel-collector-grpc.yaml)。鉴权、请求限制、TLS 边界和协议范围见英文 [运行时接收器文档](docs/RUNTIME_RECEIVER.md)。v0.10 只处理 traces，不接收 metrics、logs 或 profiles。

无需模型 API Key 即可运行四项确定性兼容性检查：

```bash
scripts/live/verify_agent_framework.sh
scripts/live/verify_dify_instrumentation.sh
scripts/live/verify_dify_runtime.sh
scripts/live/verify_mcp_runtime.sh
```

第一项运行 Microsoft Agent Framework 的真实 Agent、模型遥测和工具调用链路；第二项快速隔离执行 Dify 1.16.1 的真实 OTel handler/parser；第三项启动官方最小 Dify 应用栈；第四项运行官方 Go MCP SDK v1.7.0 的真实 stdio 客户端/服务端，通过 `server/discover` 获得稳定服务身份，并验证能力漂移和 `Agent → MCP Server → Tool` 路径策略。MCP SDK 本身不会自动发出 OTLP，本测试在应用边界插桩；`aiebom.mcp.server.*` 是明确标注的项目扩展，不冒充 OTel 标准字段。证据等级和限制见 [兼容矩阵](docs/COMPATIBILITY.md)、[v0.6 验证记录](docs/evidence/v0.6.0.md) 与 [v0.7 验证记录](docs/evidence/v0.7.0.md)。

兼容矩阵见 [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md)，每轮方向校准记录见 [docs/DIRECTION.md](docs/DIRECTION.md)。完整使用方法、架构和项目边界请查看英文 [README.md](README.md)。
