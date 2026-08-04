# AI Evidence BOM

AI Evidence BOM 是一个早期、厂商中立的验证项目：它把生成式 AI 和 Agent 的运行时遥测转换成保护隐私的证据图，并可导出 CycloneDX AI/ML BOM。

它关注的不是“代码中声明了什么”，而是：

> 实际运行时观察到了哪些 Agent、模型、工具、MCP Server、Prompt 和数据源，它们后来发生了什么变化？

当前为实验性 v0.1，不是合规认证工具、恶意软件判定工具，也无法发现未进行插桩的全部 AI 组件。

## 当前能力

- 读取 OTLP JSON 和简化的 observation JSON；
- 构建厂商中立的 Agent 证据关系图；
- 区分 `inferred`、`declared`、`observed`、`verified` 四级证据；
- 导出 CycloneDX 1.7；
- 检测模型、工具、MCP、数据源和权限变化；
- 使用 JSON 策略作为 CI 门禁；
- 使用 Ed25519 签名并验证证据文件；
- 默认只处理元数据，不保存 Prompt、响应、工具参数或工具结果。

## 快速体验

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

完整使用方法、架构和项目边界请查看英文 [README.md](README.md)。

