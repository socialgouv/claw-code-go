package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SocialGouv/claw-code-go/internal/permissions"
)

// TestExecuteBashKillsOrphanGrandchild guards against the hang where a
// grandchild process inherits stdout/stderr, outlives its bash parent
// after the timeout SIGKILL, and keeps the pipes open — wedging
// cmd.Wait() forever. The Setpgid + cmd.Cancel + WaitDelay combo must
// bring ExecuteBash back well under the 32s budget.
//
// The command spawns a bash that backgrounds a long-sleeping child
// inheriting stdout, then sleeps long enough to outlast the 30s bash
// timeout if the orphan held the pipe open. Without the fix this test
// would hang past `time` 60s. With the fix it returns inside ~32s
// (30s ctx timeout + 2s WaitDelay) with a typed timeout error.
func TestExecuteBashKillsOrphanGrandchild(t *testing.T) {
	start := time.Now()
	out, err := ExecuteBash(
		context.Background(),
		map[string]any{"command": "sleep 120 & sleep 120"},
		permissions.ModeAllow, "",
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected timeout error, got nil (output=%q)", out)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
	// 30s timeout + 2s WaitDelay = ceiling 32s; give some slack for CI.
	if elapsed > 40*time.Second {
		t.Errorf("ExecuteBash hung past WaitDelay: took %s", elapsed)
	}
}

// TestExecuteBashHonorsCallerCancel verifies that cancelling the
// caller's context propagates to the spawned bash. Pre-fix, ctx was
// silently dropped and only the internal 30s timeout could stop a
// runaway command.
func TestExecuteBashHonorsCallerCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := ExecuteBash(
		ctx,
		map[string]any{"command": "sleep 60"},
		permissions.ModeAllow, "",
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	// Should return within ~2s of cancel (200ms + WaitDelay margin).
	if elapsed > 5*time.Second {
		t.Errorf("ExecuteBash didn't honor caller cancel: took %s", elapsed)
	}
}
