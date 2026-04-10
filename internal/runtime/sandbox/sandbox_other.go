//go:build !linux

package sandbox

// unshareSupported always returns false on non-Linux platforms.
// Namespace isolation requires Linux user namespaces via `unshare`.
//
// Degradation contract: when sandbox is unavailable, the following
// capabilities are lost:
//   - Namespace isolation (pid, mount, ipc, uts, net)
//   - Network isolation
//   - Filesystem bind mount restrictions
//
// Capabilities that still work:
//   - Configuration loading and validation
//   - Container environment detection
//   - Status reporting (with fallback_reason populated)
//   - Filesystem mode tracking (informational only)
func unshareSupported() bool { return false }

func isLinux() bool { return false }
