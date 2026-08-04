#!/usr/bin/env python3
"""Emit a deterministic Microsoft Agent Framework run through OTLP/HTTP.

The model response is local and deterministic, but the Agent, chat telemetry,
function-invocation, tool-execution, and OTLP exporter paths are the real
released framework implementations.
"""

from __future__ import annotations

import asyncio
import os
from collections.abc import Awaitable, Mapping, Sequence
from typing import Any

from agent_framework import (
    Agent,
    BaseChatClient,
    ChatResponse,
    ChatResponseUpdate,
    Content,
    FunctionInvocationLayer,
    Message,
    ResponseStream,
    UsageDetails,
    tool,
)
from agent_framework.observability import ChatTelemetryLayer, configure_otel_providers
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter


class DeterministicChatClient(FunctionInvocationLayer, ChatTelemetryLayer, BaseChatClient[Any]):
    """A local chat client that exercises the framework's real telemetry layers."""

    OTEL_PROVIDER_NAME = "openai"

    def __init__(self) -> None:
        super().__init__()
        self.call_count = 0
        self.model = "gpt-5"

    def service_url(self) -> str:
        return "https://api.openai.example.test"

    def _inner_get_response(
        self,
        *,
        messages: Sequence[Message],
        stream: bool,
        options: Mapping[str, Any],
        **kwargs: Any,
    ) -> Awaitable[ChatResponse] | ResponseStream[ChatResponseUpdate, ChatResponse]:
        del messages, stream, options, kwargs
        self.call_count += 1

        async def respond() -> ChatResponse:
            if self.call_count == 1:
                return ChatResponse(
                    messages=[
                        Message(
                            role="assistant",
                            contents=[
                                Content.from_function_call(
                                    call_id="weather-call-1",
                                    name="weather.lookup",
                                    arguments='{"location":"LIVE_TOOL_ARGUMENT_MUST_NOT_LEAK"}',
                                )
                            ],
                        )
                    ],
                    model=self.model,
                    usage_details=UsageDetails(input_token_count=11, output_token_count=7),
                )
            return ChatResponse(
                messages=[Message(role="assistant", contents=["LIVE_MODEL_OUTPUT_MUST_NOT_LEAK"])],
                model=self.model,
                finish_reason="stop",
                usage_details=UsageDetails(input_token_count=5, output_token_count=9),
            )

        return respond()


@tool(name="weather.lookup", description="Look up weather", approval_mode="never_require")
def weather_lookup(location: str) -> str:
    """Return a deterministic result so no external service is required."""
    del location
    return "LIVE_TOOL_RESULT_MUST_NOT_LEAK"


async def main() -> None:
    endpoint = os.environ.get("AIEBOM_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
    configure_otel_providers(
        enable_sensitive_data=True,
        exporters=[OTLPSpanExporter(endpoint=endpoint)],
    )

    agent = Agent(
        client=DeterministicChatClient(),
        id="travel-assistant-v1",
        name="travel-assistant",
        description="Deterministic live compatibility agent",
        instructions="LIVE_SYSTEM_INSTRUCTIONS_MUST_NOT_LEAK",
        tools=[weather_lookup],
        default_options={"model": "gpt-5", "tool_choice": "auto"},
    )
    response = await agent.run("LIVE_AGENT_INPUT_MUST_NOT_LEAK")
    if response.text != "LIVE_MODEL_OUTPUT_MUST_NOT_LEAK":
        raise RuntimeError(f"unexpected framework response: {response.text!r}")

    provider = trace.get_tracer_provider()
    if not provider.force_flush(timeout_millis=10_000):
        raise RuntimeError("OpenTelemetry tracer provider did not flush in time")
    provider.shutdown()
    print("Microsoft Agent Framework live OTLP export completed")


if __name__ == "__main__":
    asyncio.run(main())
