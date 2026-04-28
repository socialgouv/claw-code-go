# Review rationale — load-bearing non-bugs

Findings raised during multi-agent code reviews of the deferred-features
sprint that turned out to be false positives. Documented here so future
reviewers don't re-flag them and waste a fix cycle.

## OTLP exporter

### `Record` after `Stop` is safe (no panic, no data loss tracking required)
**Reviewer claim:** Records after Stop create a buffer that grows
silently because the flusher goroutine has exited.

**Reality:** The buffer is bounded by `BatchSize` only on flush
trigger; idle growth is fine because the exporter object is about to
be garbage-collected after Stop returns. Late records from a
fire-and-forget callsite are dropped silently — that's the
documented contract for telemetry sinks (matches `JsonlTelemetrySink`,
`MemoryTelemetrySink`). If you need a hard "fail loud after Stop"
contract, add an `atomic.Bool` flag — but don't do it speculatively.

### Backoff sequence is correct
**Reviewer claim:** Walk through 3 attempts: is it really 1s/2s/4s?

**Reality:** Yes — `delay *= 2` happens **after** the `time.After`
sleep, so attempt 1 sleeps 1s, attempt 2 sleeps 2s, attempt 3 sleeps
4s. The expression `if attempt+1 < e.cfg.RetryAttempts` correctly
guards the last attempt from sleeping pointlessly.

## OAuth broker

### `Acquire` holds `b.mu` for the whole flow on purpose
**Reviewer claim:** Concurrent calls for different MCP servers
serialize unnecessarily.

**Reality:** The mutex protects the on-disk token store + the local
listener allocation. Concurrent flows for different servers would
need either separate brokers or a per-server mutex map; the current
design optimises for the common case (≤ 2 OAuth-protected MCP
servers per session) and avoids a more complex locking scheme. Only
revisit if a workload appears with > 5 concurrent OAuth servers.

### `runAuthCodeFlow` goroutine is not leaked
**Reviewer claim:** `srv.Serve(listener)` runs in a goroutine without
explicit cancellation hookup.

**Reality:** `defer srv.Shutdown(...)` (with a 1s ctx) runs on every
return path, which closes the listener and unblocks Serve's accept
loop. The goroutine exits within 1s of the function returning.

## SSE transport

### `Notify` does not panic on transport error
**Reviewer claim:** `httpResp.Body.Close()` on a nil response after
Do() returns an error.

**Reality:** The code returns immediately when `err != nil`, before
touching `httpResp`. The `Body.Close()` line only runs on the
success path. Comment added inline so this is unmistakable.

### Mutex held during HTTP roundtrip is intentional
**Reviewer claim:** All Send/Notify calls serialize through one
transport, hurting throughput.

**Reality:** This was the pre-existing behavior before the
SetAuthFunc addition, preserved deliberately. JSON-RPC ordering on a
single transport must be FIFO; running concurrent HTTP POSTs would
let responses interleave. If a transport-level pool is needed in the
future, the right fix is a connection pool, not lock removal.

## Installer

### File mode `mask & 0o777 | 0o600` is defensive, not buggy
**Reviewer claim:** OR-ing 0o600 defeats restrictive perms from the
tar (e.g., a tar entry with mode 0o000 becomes 0o600).

**Reality:** That's the desired behavior. A plugin tarball with
mode-0 entries would otherwise install unreadable files and brick
the plugin. The mask preserves executable bits (0o755 → 0o755) while
ensuring user read+write minimum. This is a feature, not a bug.

### `tar.TypeRegA` is deprecated but still valid
**Reviewer claim:** Using a deprecated constant.

**Reality:** Go's `archive/tar` deprecated `TypeRegA` (older "regular
file with name 'A'" header) but kept the constant for backward
compatibility. Real-world tarballs from older tools still emit it.
Removing it loses extraction support for those archives.

## CLAUDE.md loader

### `bufio.Scanner` 1 MB token cap is intentional
**Reviewer claim:** Lines longer than 1 MB silently fail.

**Reality:** A CLAUDE.md with a 1 MB single line is malformed. Failing
the parse and skipping the file is correct behavior. Documenting in
this file rather than per-callsite to keep the loader concise.

### `findAncestorClaudeMd` terminates on Windows volume roots
**Reviewer claim:** `filepath.Dir("C:\\")` returns `"C:\\"`, infinite
loop.

**Reality:** The `if parent == abs { break }` check handles exactly
this. Tested manually on Linux; Windows behavior follows the same
fixed-point property of `filepath.Dir`.

## Path traversal in extractTarGz

### Two redundant traversal checks are intentional
**Reviewer claim:** `filepath.Clean` + prefix check is duplicated by
the `filepath.Rel` check.

**Reality:** Defense in depth. The `Clean` check catches the obvious
`../foo` patterns before `filepath.Join` resolves them; the `Rel`
check catches subtler cases where `Join` might collapse paths
unexpectedly on a future Go version. Cheap to keep both — expensive
to debug a path-traversal CVE.
