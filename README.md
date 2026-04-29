# claw-code-go

<p align="center">
  <img src="assets/claw-code-go.png" alt="claw-code-go" width="360" />
</p>

<p align="center">
  <strong>Experimental fork — ingesting features from claw-code (Rust) into claw-code-go (Go)</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go" />
  <img src="https://img.shields.io/badge/Claude-claude--sonnet--4-blueviolet?style=flat-square&logo=anthropic" />
  <img src="https://img.shields.io/badge/MCP-supported-green?style=flat-square" />
  <img src="https://img.shields.io/badge/Multi--provider-Anthropic%20%7C%20OpenAI%20%7C%20Bedrock%20%7C%20Vertex%20%7C%20Foundry-orange?style=flat-square" />
  <img src="https://img.shields.io/badge/status-experimental-red?style=flat-square" />
</p>

---

> **Warning**: This is an ongoing experiment and is **not tested**. Use at your own risk.

---

## Recent changes

Highlights since the goai → claw-code-go migration landed in iterion:

- New `claw-code-go timeline --session <id>` subcommand renders a saved session's chronological events through the TUI markdown renderer. Flags: `--store <dir>`, `--format pretty|json|md`, `--limit n`. Example: `claw-code-go timeline --session demo --format md` (`internal/compat/timeline.go`).
- Typed `api.APIError` with `StatusCode` / `Retryable` so callers drive retry classification via `errors.As` instead of string parsing (`internal/api/errors.go`).
- OpenAI provider now routes `reasoning_effort` + tools through `/v1/responses` (was rejected by `/v1/chat/completions`); `internal/api/providers/openai/responses.go`.
- Real Bedrock, Vertex, and Foundry providers — no longer stubs (`pkg/api/providers/{bedrock,vertex,foundry}/provider.go`).
- Permission modes extended from 5 → 7: `ModeDontAsk` (strict allow-list) and `ModeAuto` with a pluggable `Classifier` (`internal/permissions/{mode.go,classifier.go}`).
- In-process lifecycle hooks `Runner` (Pre/PostToolUse, UserPromptSubmit, Pre/PostCompact, Stop) integrated into `runtime/conversation.go` (`internal/hooks/runner.go`).
- Shared `internal/api/{httputil,sseutil}` + `providers/openaiwire` packages, fixing args-before-id buffering and silent tool-conversion drops; tree-wide `gofmt`.

See [`CHANGELOG.md`](./CHANGELOG.md) for the full list and [`docs/parity.md`](./docs/parity.md) for the current Claude Code parity matrix.

---

## What is this?

This repo is a fork of [daolmedo/claw-code-go](https://github.com/daolmedo/claw-code-go), a Go reimplementation of [Claude Code](https://docs.anthropic.com/en/docs/claude-code) — Anthropic's agentic coding assistant.

On top of the upstream codebase, this fork ingests features from [ultraworkers/claw-code](https://github.com/ultraworkers/claw-code) — a Rust port of Claude Code that emerged after the source leak. The goal is to bring the more complete Rust implementation's capabilities into the Go codebase: hooks, plugins, MCP lifecycle, sandbox, permissions engine, runtime session management, apikit with multi-provider routing, prompt caching, and much more.

**This is an experiment, not a product.** The ported code has not been manually tested or validated beyond automated build checks.

## How was this done?

The Rust-to-Go feature porting was orchestrated by [**Iterion**](https://github.com/SocialGouv/iterion) — a workflow engine for complex multi-agent LLM pipelines, using Claude Code, Codex, and other backends.

This repo serves as the real-world example that drove Iterion's development. The full workflow configuration and writeup are available:

- [**rust_to_go_port.iter**](https://github.com/SocialGouv/iterion/blob/main/examples/rust_to_go_port.iter) — the Iterion workflow definition (DSL)
- [**rust_to_go_port.md**](https://github.com/SocialGouv/iterion/blob/main/examples/rust_to_go_port.md) — design decisions, lessons learned, and empirical data

### Run stats

| Metric | Value |
|--------|-------|
| Refinement runs | 4 (workflow iterated alongside the engine) |
| Outer loop iterations | 25+ batches across all runs |
| Commits added | 48 (47 by Iterion, 1 by Claude Code) |
| Go code generated | 37,995 lines across 173 files + 96 test files |
| New Go packages | 30+ (hooks, plugin, apikit, worker, lane, policy, recovery, sandbox, lsp, task, team...) |
| Feature parity | 100% (37/37 features) |
| Final run | 100% parity in 41 min, single batch, zero fix loops, zero interventions |
| Longest autonomous stretch | 2h25m without human intervention |
| Dual-judge verdicts | 30+ across all runs (Claude + Codex) |

### How the workflow operates

The workflow breaks the porting into dependency-ordered batches. Each batch goes through: **plan → implement → simplify → commit → test → parity scan → dual-judge review → fix loop**. Session continuity (fork/inherit) preserves KV cache across related phases. A human gate pauses for high-risk batches and auto-approves routine ones.

See the [full writeup](https://github.com/SocialGouv/iterion/blob/main/examples/rust_to_go_port.md) for details on convergence strategies, stagnation detection, and model allocation.

## Providers

Five providers are wired through `pkg/api/providers/`:

| Provider | Status | Path |
|----------|--------|------|
| Anthropic | validated end-to-end | `pkg/api/providers/anthropic` |
| OpenAI    | validated end-to-end (`/v1/chat/completions` and `/v1/responses`) | `pkg/api/providers/openai` |
| Bedrock   | available — built on `aws-sdk-go-v2`, untested in production | `pkg/api/providers/bedrock` |
| Vertex AI | available — Google ADC + canonical `MapModelID`, untested in production | `pkg/api/providers/vertex` |
| Azure Foundry | available — OpenAI-wire compatible, untested in production | `pkg/api/providers/foundry` |

Models are addressed as `<provider>/<model-id>`, e.g. `openai/gpt-5.4-mini`, `anthropic/claude-sonnet-4-6`, `bedrock/anthropic.claude-sonnet-4-6`, `vertex/claude-sonnet-4-6`.

### Running live provider tests

Each cloud provider ships a smoke test gated by the `live` Go build tag. The tests skip cleanly when the relevant credentials are not set, so the default `go test ./...` is unaffected. To run them you must opt in by passing `-tags live` and the documented env vars:

```bash
# AWS Bedrock — uses the standard AWS SDK credentials chain.
AWS_REGION=us-east-1 \
AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... \
BEDROCK_MODEL=anthropic.claude-3-5-sonnet-20241022-v2:0 \
go test -tags live -run TestLiveStreamSmokeBedrock -v ./internal/api/providers/bedrock/...

# Google Vertex AI — uses Application Default Credentials.
GOOGLE_CLOUD_PROJECT=my-project \
GOOGLE_CLOUD_REGION=us-east5 \
GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json \
VERTEX_MODEL=claude-sonnet-4-20250514 \
go test -tags live -run TestLiveStreamSmokeVertex -v ./internal/api/providers/vertex/...

# Azure AI Foundry — api-key auth, or DefaultAzureCredential when key is unset.
AZURE_OPENAI_ENDPOINT=https://my-resource.openai.azure.com \
AZURE_OPENAI_API_KEY=... \
FOUNDRY_MODEL=my-deployment \
go test -tags live -run TestLiveStreamSmokeFoundry -v ./internal/api/providers/foundry/...
```

Each test asserts only that the stream produced at least one text delta and a `message_stop` event; it does not check the exact wording of the reply. That is enough to confirm authentication, request shape, and SSE decoding all work end-to-end. Test files: `internal/api/providers/{bedrock,vertex,foundry}/provider_live_test.go`.

## Built-in tools

The `pkg/api/tools` package re-exports the built-in tools as a stable public API. Each tool is a pair of `XxxTool() api.Tool` (schema) + `ExecuteXxx(ctx, input)` (runtime).

```go
import (
    "context"
    "github.com/SocialGouv/claw-code-go/pkg/api"
    "github.com/SocialGouv/claw-code-go/pkg/api/tools"
)

defs := []api.Tool{
    tools.ReadFileTool(),
    tools.WriteFileTool(),
    tools.GlobTool(),
    tools.GrepTool(),
    tools.FileEditTool(),
    tools.WebFetchTool(),
    tools.BashTool(),
}

// Dispatch a tool call from the model:
out, err := tools.ExecuteReadFile(ctx, map[string]any{"path": "README.md"})
```

`ExecuteBash` additionally takes a `workspace string` for command validation (pass `""` to skip). The wrapper pins permissions to `ModeAllow`; gate invocations upstream (e.g. an iterion workflow's `allowed_tools` list).

## Permission modes

Defined in `internal/permissions/mode.go`, re-exported from `pkg/permissions`:

| Mode | Behavior |
|------|----------|
| `ModeAllow` | Permits all operations without prompting. |
| `ModePrompt` | Consults the ruleset; asks the prompter when no rule matches. |
| `ModeReadOnly` | Allows read-only operations only; denies writes/exec. |
| `ModeWorkspaceWrite` | Allows writes within the workspace directory; denies outside. |
| `ModeDangerFullAccess` | Allows arbitrary command execution and system access. |
| `ModeDontAsk` | Strict allow-list: denies anything not explicitly listed by `WithToolRequirement` or an allow rule. Never prompts. |
| `ModeAuto` | Delegates to a `Classifier` (default safe-list permits read-only ops, prompts on writes); custom classifiers via `WithClassifier`. |

CLI aliases (`default`, `accept-edits`, `bypass`, `plan`) resolve to the modes above — see `ParsePermissionMode`.

## Lifecycle hooks (in-process)

`internal/hooks/runner.go` provides a programmatic `Runner` for `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `UserPromptSubmit`, `PreCompact`, `PostCompact`, and `Stop`. First non-`Continue` decision wins; panics and errors are logged and treated as `Continue`.

```go
runner := hooks.NewRunner()
runner.Register(hooks.PreToolUse, func(ctx context.Context, hctx hooks.Context) (hooks.Decision, error) {
    if hctx.ToolName == "bash" { return hooks.Decision{Action: hooks.ActionBlock, Reason: "no shell"}, nil }
    return hooks.Decision{Action: hooks.ActionContinue}, nil
})
decision, _ := runner.Fire(ctx, hooks.Context{Event: hooks.PreToolUse, ToolName: "bash"})
```

The Runner is wired into `internal/runtime/conversation.go`; nil runners are a no-op.

## Status

**Experimental.** The code compiles and passes `go vet`, but has not been manually tested. The latest commit (Anthropic prompt caching with `cache_control` breakpoints) was done directly with Claude Code outside the Iterion workflow.

Contributions, testing, and feedback are welcome.

## Credits

- [**daolmedo/claw-code-go**](https://github.com/daolmedo/claw-code-go) — the original Go port of Claude Code (upstream)
- [**ultraworkers/claw-code**](https://github.com/ultraworkers/claw-code) — the Rust port of Claude Code (feature source)
- [**Iterion**](https://github.com/SocialGouv/iterion) — the workflow orchestration engine that performed the porting

## License

MIT
