package hooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRunner_FiresAllHandlers(t *testing.T) {
	r := NewRunner(WithLogger(io.Discard))

	var calls atomic.Int32
	for i := 0; i < 3; i++ {
		r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
			calls.Add(1)
			return Decision{Action: ActionContinue}, nil
		})
	}

	dec, err := r.Fire(context.Background(), Context{Event: PreToolUse, ToolName: "bash"})
	if err != nil {
		t.Fatalf("Fire: unexpected error: %v", err)
	}
	if dec.Action != ActionContinue {
		t.Fatalf("expected ActionContinue, got %v", dec.Action)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected all 3 handlers to fire, got %d", got)
	}
}

func TestRunner_FirstBlockWins(t *testing.T) {
	r := NewRunner(WithLogger(io.Discard))

	var (
		first  atomic.Int32
		second atomic.Int32
		third  atomic.Int32
	)

	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		first.Add(1)
		return Decision{Action: ActionContinue}, nil
	})
	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		second.Add(1)
		return Decision{Action: ActionBlock, Reason: "no thanks"}, nil
	})
	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		third.Add(1)
		return Decision{Action: ActionContinue}, nil
	})

	dec, err := r.Fire(context.Background(), Context{Event: PreToolUse, ToolName: "bash"})
	if err != nil {
		t.Fatalf("Fire: unexpected error: %v", err)
	}
	if dec.Action != ActionBlock {
		t.Fatalf("expected ActionBlock, got %v", dec.Action)
	}
	if dec.Reason != "no thanks" {
		t.Fatalf("expected reason to propagate, got %q", dec.Reason)
	}
	if first.Load() != 1 {
		t.Errorf("first handler should have fired exactly once, got %d", first.Load())
	}
	if second.Load() != 1 {
		t.Errorf("second handler should have fired exactly once, got %d", second.Load())
	}
	if third.Load() != 0 {
		t.Errorf("third handler should NOT have fired after Block, got %d", third.Load())
	}
}

func TestRunner_HandlerErrorIsContinue(t *testing.T) {
	var buf bytes.Buffer
	r := NewRunner(WithLogger(&buf))

	var afterErr atomic.Int32

	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		return Decision{}, errors.New("boom")
	})
	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		afterErr.Add(1)
		return Decision{Action: ActionContinue}, nil
	})

	dec, err := r.Fire(context.Background(), Context{Event: PreToolUse, ToolName: "bash"})
	if err != nil {
		t.Fatalf("Fire: unexpected error: %v", err)
	}
	if dec.Action != ActionContinue {
		t.Fatalf("expected ActionContinue after handler error, got %v", dec.Action)
	}
	if afterErr.Load() != 1 {
		t.Fatalf("subsequent handler should have run after error, got %d", afterErr.Load())
	}
	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("expected handler error to be logged, log was: %q", buf.String())
	}
}

func TestRunner_PanicIsContinue(t *testing.T) {
	var buf bytes.Buffer
	r := NewRunner(WithLogger(&buf))

	var afterPanic atomic.Int32

	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		panic("kaboom")
	})
	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		afterPanic.Add(1)
		return Decision{Action: ActionContinue}, nil
	})

	dec, err := r.Fire(context.Background(), Context{Event: PreToolUse, ToolName: "bash"})
	if err != nil {
		t.Fatalf("Fire: unexpected error: %v", err)
	}
	if dec.Action != ActionContinue {
		t.Fatalf("expected ActionContinue after panic, got %v", dec.Action)
	}
	if afterPanic.Load() != 1 {
		t.Fatalf("subsequent handler should have run after panic, got %d", afterPanic.Load())
	}
	if !strings.Contains(buf.String(), "kaboom") {
		t.Errorf("expected panic to be logged, log was: %q", buf.String())
	}
}

func TestRunner_NilRunnerIsNoop(t *testing.T) {
	var r *Runner
	dec, err := r.Fire(context.Background(), Context{Event: PreToolUse})
	if err != nil {
		t.Fatalf("Fire on nil runner: unexpected error: %v", err)
	}
	if dec.Action != ActionContinue {
		t.Fatalf("nil runner should return Continue, got %v", dec.Action)
	}
	// Register on a nil runner must not panic.
	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		return Decision{}, nil
	})
}

func TestRunner_PerEventIsolation(t *testing.T) {
	r := NewRunner(WithLogger(io.Discard))

	var preCalls, postCalls atomic.Int32

	r.Register(PreToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		preCalls.Add(1)
		return Decision{Action: ActionContinue}, nil
	})
	r.Register(PostToolUse, func(ctx context.Context, hctx Context) (Decision, error) {
		postCalls.Add(1)
		return Decision{Action: ActionContinue}, nil
	})

	if _, err := r.Fire(context.Background(), Context{Event: PreToolUse}); err != nil {
		t.Fatal(err)
	}
	if preCalls.Load() != 1 || postCalls.Load() != 0 {
		t.Fatalf("PreToolUse fired wrong handlers: pre=%d post=%d", preCalls.Load(), postCalls.Load())
	}

	if _, err := r.Fire(context.Background(), Context{Event: PostToolUse}); err != nil {
		t.Fatal(err)
	}
	if preCalls.Load() != 1 || postCalls.Load() != 1 {
		t.Fatalf("PostToolUse fired wrong handlers: pre=%d post=%d", preCalls.Load(), postCalls.Load())
	}
}

func TestRunner_ConcurrentRegister(t *testing.T) {
	// Race-detector test: many goroutines register and fire simultaneously.
	r := NewRunner(WithLogger(io.Discard))

	const workers = 32
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				r.Register(Event(id%7), func(ctx context.Context, hctx Context) (Decision, error) {
					return Decision{Action: ActionContinue}, nil
				})
			}
		}(w)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_, _ = r.Fire(context.Background(), Context{Event: Event(id % 7)})
			}
		}(w)
	}

	wg.Wait()
}

func TestEvent_String(t *testing.T) {
	cases := map[Event]string{
		PreToolUse:         "PreToolUse",
		PostToolUse:        "PostToolUse",
		PostToolUseFailure: "PostToolUseFailure",
		UserPromptSubmit:   "UserPromptSubmit",
		PreCompact:         "PreCompact",
		PostCompact:        "PostCompact",
		Stop:               "Stop",
	}
	for e, want := range cases {
		if got := e.String(); got != want {
			t.Errorf("Event(%d).String() = %q, want %q", int(e), got, want)
		}
	}
	// Unknown event prints fallback.
	got := Event(999).String()
	want := fmt.Sprintf("Event(%d)", 999)
	if got != want {
		t.Errorf("Event(999).String() = %q, want %q", got, want)
	}
}
