# Parity matrix vs Claude Code

Snapshot as of 2026-04-28, after the multi-phase port session.

Rating legend:

- **COMPLETE** — feature is wired end-to-end and exercised by tests or live runs.
- **PARTIAL** — the substrate ships, but at least one user-visible surface (broker, exporter, UI, etc.) is still pending. See notes.
- **MISSING** — placeholder or types may exist, but the user-visible feature is not callable.

| Capability axis | claude_code (Anthropic CLI) | claw-code-go | Rating | Notes |
|-----------------|------------------------------|--------------|--------|-------|
| Built-in tool surface (read_file, write_file, bash, glob, grep, file_edit, web_fetch) | yes | yes | COMPLETE | `pkg/api/tools/builtins.go`, `internal/tools/*.go`. Bash gates on the permissions package. |
| Slash commands / skills | yes | yes | COMPLETE | Skill registry: `internal/tools/skill.go`. Slash dispatcher: `internal/commands/dispatcher.go`, `internal/commands/slash_test.go`. |
| MCP (stdio + SSE + websocket clients, server SDK) | yes | yes | COMPLETE | Transport: `internal/mcp/{stdio,sse,websocket,transport_rpc}.go`. Server SDK: `internal/mcp/sdk_server.go`. Lifecycle: `internal/mcp/lifecycle.go`. OAuth broker plugs into `TransportConfig.AuthFunc` (see below). |
| MCP OAuth (authorize, refresh, store) | yes | yes | COMPLETE | Auth-code + PKCE broker on top of the atomic disk storage: `internal/mcp/oauth/{broker.go,pkce.go,storage.go}`. Public façade re-exports `Broker`, `NewBroker`, `Token`, `Storage`, and the typed `ErrReauthRequired` for headless callers: `pkg/api/mcp/oauth/oauth.go`. Tests: `internal/mcp/oauth/{broker_test.go,pkce_test.go}` (RFC 6749 / 7636 happy path, refresh rotation, invalid_grant → ErrReauthRequired, transient 5xx, state-mismatch CSRF reject, revoke clears local cache on remote failure). Transports consume the broker via `broker.BearerHeaderFunc(cfg)` → `TransportConfig.AuthFunc`. |
| Lifecycle hooks (PreToolUse, PostToolUse, UserPromptSubmit, PreCompact, PostCompact, Stop, Plugin Pre/Post Install + Uninstall) | yes | yes | COMPLETE | In-process Runner shipped and integrated: `internal/hooks/runner.go`, used by `internal/runtime/conversation.go` and `internal/runtime/compact.go`. Conversation `context.Context` now flows into both `Runner.Fire` and the shell `HookRunner.Run*` methods, so cancellation propagates to lifecycle handlers and to running hook scripts (`exec.CommandContext`). Plugin lifecycle hooks (`PrePluginInstall`, `PostPluginInstall`, `PrePluginUninstall`, `PostPluginUninstall`) fire from `plugin/manager.go` via the `WithHooks` option; a Pre Block aborts the operation, Post fires on success and failure with `Plugin.Error` set on failure. |
| Session (jsonl persistence, fork, inherit) | yes | yes | COMPLETE | `internal/runtime/session.go`, `internal/runtime/session_jsonl.go`, `internal/runtime/session_store.go`. JSONL turn-by-turn replay validated. |
| Sub-agents / team delegation | yes | yes | COMPLETE | `internal/runtime/team/registry.go`, `internal/tools/agent_test.go`, `internal/tools/team_tools_test.go`. |
| Providers — anthropic + openai | yes | yes | COMPLETE | `pkg/api/providers/{anthropic,openai}`. OpenAI covers both `/v1/chat/completions` and `/v1/responses` (`internal/api/providers/openai/responses.go`); the responses translator handles parallel reasoning + tool_calls without dropping interleaved text or argument deltas. |
| Providers — Bedrock / Vertex / Foundry | yes | available, live tests in repo (gated by env) | PARTIAL | Real implementations: `pkg/api/providers/{bedrock,vertex,foundry}/provider.go` + `internal/api/providers/{bedrock,vertex,foundry}`. Compile + unit tests pass. Live smoke tests under build tag `live`: `internal/api/providers/{bedrock,vertex,foundry}/provider_live_test.go` — skip cleanly without creds, exercise auth + streaming when the documented env vars are set (see "Running live provider tests" in the README). |
| Permissions (mode + allow/deny rules + prompter) | yes (5 modes) | yes (7 modes + Classifier) | COMPLETE | All 7 modes shipped: `internal/permissions/mode.go`. `Classifier` interface: `internal/permissions/classifier.go`. Two classifiers ship: the default rule-based `RuleClassifier` (read-only safe-list) and a small-model `LLMClassifier` (`internal/permissions/llm_classifier.go`) that mirrors what Claude Code uses, with a TTL+FIFO decision cache and a fail-safe-to-Ask invariant on transport errors. Pre-wired manager helper: `NewLLMClassifierManager`. |
| Compaction (auto-trim history, preserve tool-call invariants) | yes | yes | COMPLETE | `internal/runtime/compact.go`, `internal/runtime/compact_test.go`. PreCompact/PostCompact hooks wired. |
| Agent SDK (programmatic conversation loop) | yes | yes | COMPLETE | `pkg/api/client.go` (`StreamResponse`), `internal/runtime/conversation.go`. Used by iterion via `model/generation.go`. |
| Vision / computer use | yes | partial | MISSING | `api.ImageSource` types in place: `internal/api/types.go`, `pkg/api/types.go`. Screenshot/click/typing tools are not implemented. |
| Telemetry (event taxonomy + exporter) | yes (OTLP) | partial | PARTIAL | Event types: `internal/runtime/events.go`. Internal client telemetry: `internal/api/client_telemetry_test.go`. No OTLP / OpenTelemetry exporter — current sink is stderr / JSONL session log. |
| Session timeline UI | yes (TUI render) | partial | PARTIAL | TUI primitives: `internal/tui/`. Session JSONL captures every turn, but no `claw-code timeline` render command exists yet. |
| Plugin marketplace (remote discovery + install) | yes | partial | MISSING | Local plugin registry, manifest, and tool wiring shipped: `plugin/{manager.go,manifest.go,registry.go,tool.go}`. Remote marketplace / signed-manifest fetch is not wired. |

## Quick guidance for contributors

If you're adding a feature, the file:line citations above point to the package you'll most likely extend. For provider-touching work, prefer reusing `internal/api/httputil` and `internal/api/sseutil` rather than re-rolling HTTP/SSE plumbing — that's why they were extracted.

If you're chasing a "PARTIAL" rating to "COMPLETE":

- **Telemetry → OTLP**: add an exporter that subscribes to events emitted from `internal/runtime/events.go`.
- **Session UI**: read `internal/runtime/session_jsonl.go` and render through `internal/tui`.
- **Vision / computer use**: implement screenshot/click tools that emit `api.ImageSource` content blocks.
- **Plugin marketplace**: layer a remote fetch + manifest verification on top of `plugin/registry.go`.
