#!/usr/bin/env python3
"""Execute Dify's real OTel handler/parsers and export their spans over OTLP.

This is an isolated instrumentation check, not a complete Dify deployment. It
loads the implementation directly from a pinned Dify checkout and replaces only
the surrounding application/Graphon domain objects with deterministic stubs.
"""

from __future__ import annotations

import os
import subprocess
import sys
import types
from enum import Enum
from pathlib import Path
from types import SimpleNamespace

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor


EXPECTED_DIFY_VERSION = "1.16.1"
EXPECTED_DIFY_REVISION = "3ada29bbe06a33b9679b30f37a995562118aa173"


def install_host_stubs(api_root: Path) -> None:
    """Provide only the host types needed to execute Dify's parser modules."""

    class BuiltinNodeTypes(str, Enum):
        LLM = "llm"
        KNOWLEDGE_RETRIEVAL = "knowledge-retrieval"
        TOOL = "tool"

    class WorkflowNodeExecutionMetadataKey(str, Enum):
        TOOL_INFO = "tool_info"

    class File:
        def to_dict(self) -> dict[str, object]:
            return {}

    class Segment:
        value: object = None

    class GraphNodeEventBase:
        pass

    class Node:
        pass

    class ToolNodeData:
        def __init__(self, provider_type: str) -> None:
            self.provider_type = SimpleNamespace(value=provider_type)

    configs = types.ModuleType("configs")
    configs.dify_config = SimpleNamespace(ENTERPRISE_ENABLED=False, ENTERPRISE_INCLUDE_CONTENT=False)
    sys.modules["configs"] = configs

    # Import only the implementation modules under test. Dify's package-level
    # initializers also wire Celery/Flask runtime hooks, which belong to the full
    # deployment check rather than this isolated instrumentation check.
    for name, path in (
        ("extensions.otel", api_root / "extensions/otel"),
        ("extensions.otel.parser", api_root / "extensions/otel/parser"),
    ):
        package = types.ModuleType(name)
        package.__path__ = [str(path)]  # type: ignore[attr-defined]
        sys.modules[name] = package

    modules: dict[str, types.ModuleType] = {}
    for name in (
        "graphon",
        "graphon.nodes",
        "graphon.nodes.base",
        "graphon.nodes.tool",
    ):
        modules[name] = types.ModuleType(name)
        modules[name].__path__ = []  # type: ignore[attr-defined]

    enums = types.ModuleType("graphon.enums")
    enums.BuiltinNodeTypes = BuiltinNodeTypes
    enums.WorkflowNodeExecutionMetadataKey = WorkflowNodeExecutionMetadataKey
    file_module = types.ModuleType("graphon.file")
    file_module.File = File
    events = types.ModuleType("graphon.graph_events")
    events.GraphNodeEventBase = GraphNodeEventBase
    node_module = types.ModuleType("graphon.nodes.base.node")
    node_module.Node = Node
    tool_entities = types.ModuleType("graphon.nodes.tool.entities")
    tool_entities.ToolNodeData = ToolNodeData
    variables = types.ModuleType("graphon.variables")
    variables.Segment = Segment

    modules.update(
        {
            "graphon.enums": enums,
            "graphon.file": file_module,
            "graphon.graph_events": events,
            "graphon.nodes.base.node": node_module,
            "graphon.nodes.tool.entities": tool_entities,
            "graphon.variables": variables,
        }
    )
    sys.modules.update(modules)


def source_revision(api_root: Path) -> str:
    result = subprocess.run(
        ["git", "-C", str(api_root.parent), "rev-parse", "HEAD"],
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout.strip()


def verify_source(api_root: Path) -> str:
    required = (
        api_root / "extensions/otel/decorators/handlers/workflow_app_runner_handler.py",
        api_root / "extensions/otel/parser/llm.py",
        api_root / "extensions/otel/parser/tool.py",
        api_root / "pyproject.toml",
    )
    missing = [str(path) for path in required if not path.is_file()]
    if missing:
        raise RuntimeError(f"Dify checkout is incomplete: {', '.join(missing)}")

    import tomllib

    with (api_root / "pyproject.toml").open("rb") as handle:
        version = tomllib.load(handle)["project"]["version"]
    if version != EXPECTED_DIFY_VERSION:
        raise RuntimeError(f"expected Dify {EXPECTED_DIFY_VERSION}, found {version}")
    revision = source_revision(api_root)
    if revision != EXPECTED_DIFY_REVISION:
        raise RuntimeError(f"expected Dify revision {EXPECTED_DIFY_REVISION}, found {revision}")
    return revision


def main() -> None:
    api_root = Path(os.environ["AIEBOM_DIFY_API_ROOT"]).resolve()
    revision = verify_source(api_root)
    sys.path.insert(0, str(api_root))
    install_host_stubs(api_root)

    from extensions.otel.decorators.handlers.workflow_app_runner_handler import WorkflowAppRunnerHandler
    from extensions.otel.parser.llm import LLMNodeOTelParser
    from extensions.otel.parser.tool import ToolNodeOTelParser
    from graphon.enums import WorkflowNodeExecutionMetadataKey
    from graphon.nodes.tool.entities import ToolNodeData

    endpoint = os.environ.get("AIEBOM_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
    resource = Resource.create(
        {
            "service.name": "dify-api",
            "service.version": f"dify-{EXPECTED_DIFY_VERSION}",
            "dify.source.revision": revision,
        }
    )
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(SimpleSpanProcessor(OTLPSpanExporter(endpoint=endpoint)))
    trace.set_tracer_provider(provider)
    tracer = provider.get_tracer("dify.extensions.otel.live-check", EXPECTED_DIFY_VERSION)

    runner = SimpleNamespace(
        application_generate_entity=SimpleNamespace(
            user_id="sanitized-user",
            stream=False,
            app_config=SimpleNamespace(
                app_id="travel-assistant-v1",
                tenant_id="sanitized-tenant",
                workflow_id="travel-workflow-v1",
            ),
        )
    )

    def execute_workflow(self: object) -> str:
        del self
        llm_node = SimpleNamespace(id="llm-node", execution_id="", node_type="llm")
        llm_result = SimpleNamespace(
            process_data={
                "model_name": "gpt-5",
                "model_provider": "openai",
                "prompts": [{"role": "system", "text": "DIFY_PROMPT_MUST_NOT_LEAK"}],
                "usage": {"prompt_tokens": 12, "completion_tokens": 8, "total_tokens": 20},
            },
            inputs={"query": "DIFY_INPUT_MUST_NOT_LEAK"},
            outputs={"text": "DIFY_OUTPUT_MUST_NOT_LEAK", "finish_reason": "stop"},
            metadata={},
        )
        with tracer.start_as_current_span("llm") as span:
            LLMNodeOTelParser().parse(
                node=llm_node,
                span=span,
                error=None,
                result_event=SimpleNamespace(node_run_result=llm_result),
            )

        tool_node = SimpleNamespace(
            id="tool-node",
            execution_id="",
            node_type="tool",
            title="weather.lookup",
            _node_data=ToolNodeData("builtin"),
        )
        tool_result = SimpleNamespace(
            process_data={},
            inputs={"location": "DIFY_TOOL_ARGUMENT_MUST_NOT_LEAK"},
            outputs={"forecast": "DIFY_TOOL_RESULT_MUST_NOT_LEAK"},
            metadata={WorkflowNodeExecutionMetadataKey.TOOL_INFO: {"safe": "metadata"}},
        )
        with tracer.start_as_current_span("tool") as span:
            ToolNodeOTelParser().parse(
                node=tool_node,
                span=span,
                error=None,
                result_event=SimpleNamespace(node_run_result=tool_result),
            )
        return "ok"

    result = WorkflowAppRunnerHandler().wrapper(tracer, execute_workflow, runner)
    if result != "ok":
        raise RuntimeError(f"unexpected Dify handler result: {result!r}")
    if not provider.force_flush(timeout_millis=10_000):
        raise RuntimeError("OpenTelemetry tracer provider did not flush in time")
    provider.shutdown()
    print(f"Dify {EXPECTED_DIFY_VERSION} instrumentation OTLP export completed ({revision[:12]})")


if __name__ == "__main__":
    main()
