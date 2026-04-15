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
  <img src="https://img.shields.io/badge/Multi--provider-Anthropic%20%7C%20OpenAI-orange?style=flat-square" />
  <img src="https://img.shields.io/badge/status-experimental-red?style=flat-square" />
</p>

---

> **Warning**: This is an ongoing experiment and is **not tested**. Use at your own risk.

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

## Status

**Experimental.** The code compiles and passes `go vet`, but has not been manually tested. The latest commit (Anthropic prompt caching with `cache_control` breakpoints) was done directly with Claude Code outside the Iterion workflow.

Contributions, testing, and feedback are welcome.

## Credits

- [**daolmedo/claw-code-go**](https://github.com/daolmedo/claw-code-go) — the original Go port of Claude Code (upstream)
- [**ultraworkers/claw-code**](https://github.com/ultraworkers/claw-code) — the Rust port of Claude Code (feature source)
- [**Iterion**](https://github.com/SocialGouv/iterion) — the workflow orchestration engine that performed the porting

## License

MIT
