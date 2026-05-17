# claw-code-go — iterion-touching audit, 2026-05-17

Author: companion review to iterion's `docs/reviews/codebase-2026-05-17.md`.
Scope: the parts of claw-code-go that iterion calls — `internal/api/*`, `internal/runtime/compact.go`, `internal/apikit/*`, `internal/api/providers/{openai,openaiwire,bedrock,vertex,anthropic}/*`. **Excluded**: `internal/runtime/conversation.go` (iterion runs its own loop) and `internal/tools/bash.go` (iterion uses its own Bash executor).
Branch: `bugfix/iterion-audit-2026-05-17` (worktree at `.works/claw-code-go-iterion-audit-2026-05-17/`).
Vendor alignment confirmed: iterion's `go.sum` pin = master HEAD `b4b2f7c`.

---

## TL;DR

**11 findings** (0 P0, 4 P1, 5 P2, 2 P3). No P0: the SDK's HTTP/SSE core is well-disciplined (typed `APIError`, exponential backoff retry, context cancellation in every blocking select, immutable body bytes per attempt). The P1 cluster is around two correctness/observability gaps in the OpenAI Responses SSE translator + one retry-path inconsistency in `client.go` + one async-cache-refresh anti-stampede issue.

| Axis | P0 | P1 | P2 | P3 | Note |
|---|---|---|---|---|---|
| OpenAI Responses SSE translation | 0 | 2 | 2 | 1 | high churn area |
| HTTP client / retry / errors | 0 | 1 | 0 | 0 | typed APIError gap |
| Live registry / cache | 0 | 1 | 1 | 0 | async-refresh stampede |
| Compactor + prompt cache | 0 | 0 | 2 | 1 | role pollution + lock pattern |

---

## 1. Methodology

Single focused `code-reviewer` agent against the iterion-touching file list (extracted from a prior dependency-mapping Explore pass). Files inspected:
- `internal/api/client.go`, `auth.go`, `types.go`, `sse.go`
- `internal/api/providers/openai/responses.go`
- `internal/api/providers/openaiwire/*` (convert.go, stream.go)
- `internal/api/providers/{bedrock,vertex,anthropic}/provider.go`
- `internal/runtime/compact.go`
- `internal/apikit/{model_registry_live.go, prompt_cache.go, prompt_cache_fingerprint.go, errors.go, retry.go}`

All recently-shipped fixes verified in current code (b4b2f7c, 98731b5, c67d13f, c1cdea5, 373d944, 35aa371, 5b66c4f).

---

## 2. Findings index

| ID | Sev | File | Title |
|---|---|---|---|
| F-CC-1 | P1 | openai/responses.go:528 | function_call deltas can be silently lost if start event omits call_id/name |
| F-CC-2 | P1 | api/client.go:142 | Transport-error retry path drops typed `*APIError` |
| F-CC-3 | P1 | apikit/model_registry_live.go:260 | Fetched cache `liveFetchLast` bumped on failure → 24h dark window |
| F-CC-4 | P1 | api/client.go:113 | `anthropic-beta` set unconditionally; OAuth sessions may 400 |
| F-CC-5 | P2 | openai/responses.go:600 | function_call_arguments.delta on unknown item id silently dropped |
| F-CC-6 | P2 | openaiwire/stream.go:78 | Strict `data: ` prefix drops valid SSE frames without space |
| F-CC-7 | P2 | api/types.go:133 | `is_injected` leaks onto wire — future strict-mode rejection |
| F-CC-8 | P2 | runtime/compact.go:289 | `GetContinuationMessage` emits role `system` inside messages[] |
| F-CC-9 | P2 | apikit/prompt_cache.go:144 | `LookupCompletion` lock/unlock/lock pattern races on concurrent calls |
| F-CC-10 | P3 | openai/responses.go:611 | `response.completed` doesn't break — waits for EOF |
| F-CC-11 | P3 | runtime/compact.go:236 | `recent` aliases caller slice in pure variant (defence-in-depth) |

---

## 7. Shipped vs deferred

| ID | Sev | Status | Notes |
|---|---|---|---|
| F-CC-2 | P1 | **shipped** | transport-error retry exhaustion now returns typed `*APIError` |
| F-CC-3 | P1 | **shipped** | live registry: don't bump `liveFetchLast` on fetch failure |
| F-CC-1 | P1 | deferred | function_call deltas without identity silently lost — needs OpenAI Responses-API frame semantics design (fail-fast vs reconciliation in `response.completed`) |
| F-CC-4 | P1 | deferred | anthropic-beta unconditional on OAuth — needs auth-source gating + provider-side validation contract clarification |
| F-CC-5 | P2 | deferred | function_call_arguments.delta on unknown item id silently dropped |
| F-CC-6 | P2 | deferred | strict "data: " prefix drops valid SSE frames without space (Ollama / vLLM compat) |
| F-CC-7 | P2 | deferred | `is_injected` leaks onto wire — needs MarshalJSON custom on Message |
| F-CC-8 | P2 | deferred | GetContinuationMessage role "system" inside messages[] — needs Anthropic top-level system extraction |
| F-CC-9 | P2 | deferred | LookupCompletion lock/unlock/lock race on prompt-cache stats |
| F-CC-10 | P3 | deferred | response.completed doesn't break loop |
| F-CC-11 | P3 | deferred | recent slice aliasing in CompactSessionPure |
