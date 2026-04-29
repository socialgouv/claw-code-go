# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Changes since `bf21311` (last stable commit before the multi-phase port session of 2026-04-28).

### Added

- Live test coverage for Bedrock, Vertex, and Foundry providers under build tag `live`. Each test instantiates the real provider, runs a short streaming request, and asserts that at least one text delta plus a stop event are received; tests skip cleanly when the documented per-provider env vars are missing, so `go test ./...` (no tag) is unaffected. (`internal/api/providers/{bedrock,vertex,foundry}/provider_live_test.go`)
- Typed `api.APIError` returned by provider clients on non-2xx responses, exposing `Provider`, `StatusCode`, `Message`, `Body`, and `Retryable`. Callers drive retry classification via `errors.As` instead of string parsing. `IsRetryableStatus` covers 408/409/429/5xx. (`internal/api/errors.go`, commit 2574d7f)
- OpenAI provider routes `reasoning_effort` + tools through `/v1/responses` and translates its SSE event stream; `/v1/chat/completions` is kept for the legacy path. (`internal/api/providers/openai/responses.go`, commit 14716b8)
- Real AWS Bedrock provider built on `aws-sdk-go-v2` (replaces the stub). (`pkg/api/providers/bedrock/provider.go` + `internal/api/providers/bedrock`, commit 3ce3cea)
- Real Google Vertex AI and Azure Foundry providers. Vertex uses ADC + canonical `MapModelID`; Foundry reuses the OpenAI wire format. (`pkg/api/providers/{vertex,foundry}/provider.go` + `internal/api/providers/{vertex,foundry}`, commit 4f6a013)
- `ModeDontAsk` (strict allow-list, never prompts) and `ModeAuto` (delegates to a `Classifier`) added to `PermissionMode`. Default `RuleClassifier` permits a small read-only safe-list and prompts otherwise; custom classifiers can be plugged in via `WithClassifier`. (`internal/permissions/mode.go`, `internal/permissions/classifier.go`, commit 6f29983)
- In-process lifecycle hooks `Runner` with sequential, deterministic dispatch and "first non-Continue wins" semantics. Events: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `UserPromptSubmit`, `PreCompact`, `PostCompact`, `Stop`. Integrated into `runtime/conversation.go` and `runtime/compact.go`; nil Runner is a documented no-op. (`internal/hooks/runner.go`, commit bd616bf)
- `api.ImageSource` type for Anthropic vision content blocks; re-exported from `pkg/api`. (`internal/api/types.go`, `pkg/api/types.go`, commit 52392bd)
- MCP OAuth Authorization Code + PKCE broker on top of the atomic disk-backed token storage. The broker drives the loopback callback flow (RFC 7636 S256 challenge), exchanges the code at the token endpoint, and silently refreshes near-expiry tokens. Transports plug it in via `broker.BearerHeaderFunc(cfg)` → `TransportConfig.AuthFunc`, so a stale access token triggers an automatic refresh on the next request. Headless callers use `Broker.AcquireNoninteractive`, which returns the typed `ErrReauthRequired` instead of opening a browser when refresh is impossible (no refresh_token, or AS replied `invalid_grant`); transient errors (network, 5xx) propagate verbatim so retry logic can distinguish them. The atomic disk-backed token storage is the persistence layer for the broker. Public façade in `pkg/api/mcp/oauth` re-exports `Broker`, `NewBroker`, `Token`, `Storage`, and `ErrReauthRequired`. (`internal/mcp/oauth/{broker.go,broker_test.go,pkce.go,pkce_test.go,storage.go}`, `pkg/api/mcp/oauth/oauth.go`, commit 4a42881 for the underlying storage layer)
- Shared `internal/api/httputil` and `internal/api/sseutil` packages plus a dedicated `providers/openaiwire` package; eliminates duplicated request/SSE plumbing across providers. (commit 80f5a8c)

### Fixed

- Tree-wide `gofmt -w` pass to normalize formatting across the module. (commit f409e88)
- BUG 1 — `vertex.MapModelID` now correctly maps canonical Claude model IDs to Vertex AI model strings (was returning the input verbatim for several IDs). (`internal/api/providers/vertex/provider.go`, commit 80f5a8c)
- BUG 6 — `convertTools` no longer silently drops tools with unsupported field shapes; unsupported tools now produce an explicit error so callers can surface the misconfiguration. (`internal/api/providers/openai/responses.go`, commit 80f5a8c)
- BUG 7 — Tool-call args-before-id buffering corrected via the new `sseutil.ToolCallAccumulator`; assistant turns no longer drop arguments that arrive before the tool-call id in OpenAI streams. (`internal/api/sseutil/toolcall.go`, commit 80f5a8c)
- BUGs 2 / 3 — `/v1/responses` interleave assembly for parallel reasoning + tool calls. Each output item now keeps its own block index across the entire stream; opening a `function_call` no longer prematurely closes a sibling `message` text block, so trailing text deltas that resume after a tool call are preserved instead of being dropped on a closed block. Parallel `function_call` items keep arguments correctly partitioned by `item_id` even when their argument deltas are fully interleaved on the wire. (`internal/api/providers/openai/responses.go`)
- 7 additional quality items rolled into the same refactor (validation gaps, error-wrapping consistency, dead branches in SSE decoding). (commit 80f5a8c)
- BUG 4 — Conversation `context.Context` now flows into both the in-process `internal/hooks` `Runner.Fire` and the shell `hooks.HookRunner.Run*` methods. The shell runner uses `exec.CommandContext`, so cancelling the conversation kills any in-flight hook script and surfaces a `Cancelled` result instead of letting the hook outlive the session. (`hooks/runner.go`, `hooks/exec.go`, `internal/hooks/runner.go`, `internal/runtime/conversation.go`)

### Deferred

- Computer-use tools — `ImageSource` types are in place but the screenshot/click/typing tool surface is not yet wired.
- Session timeline UI — the JSONL session store captures all data, but no CLI render of the timeline exists yet.
- OTLP exporter — telemetry event types are defined, but there is no exporter to OpenTelemetry collectors.
- Plugin marketplace — plugin manifests + local registry exist; remote discovery / install is not wired.

[Unreleased]: https://github.com/SocialGouv/claw-code-go/compare/bf21311...HEAD
