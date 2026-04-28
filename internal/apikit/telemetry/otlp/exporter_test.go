package otlp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SocialGouv/claw-code-go/internal/apikit"
)

func makeExporter(t *testing.T, srv *httptest.Server, opts ...func(*Config)) *Exporter {
	t.Helper()
	cfg := Config{
		Endpoint:      srv.URL,
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
		HTTPClient:    srv.Client(),
		RetryAttempts: 3,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	exp, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := exp.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = exp.Stop(ctx)
	})
	return exp
}

// capturedReqs is a thread-safe collector for httptest payloads.
type capturedReqs struct {
	mu   sync.Mutex
	data [][]byte
}

func (c *capturedReqs) add(b []byte) {
	c.mu.Lock()
	c.data = append(c.data, b)
	c.mu.Unlock()
}

func (c *capturedReqs) snapshot() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.data))
	copy(out, c.data)
	return out
}

// captureRequests builds an httptest server that records every incoming
// payload and returns the configured status code each time.
func captureRequests(t *testing.T, status int) (*httptest.Server, *capturedReqs) {
	t.Helper()
	capt := &capturedReqs{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capt.add(body)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, capt
}

func TestExporter_BatchesAndFlushesOnSize(t *testing.T) {
	srv, capt := captureRequests(t, http.StatusOK)
	exp := makeExporter(t, srv, func(c *Config) {
		c.BatchSize = 5
		c.FlushInterval = 1 * time.Hour // disable timer-driven flush
	})

	// Six events should trigger one size-driven flush of the first 5.
	for i := 0; i < 6; i++ {
		exp.Record(apikit.TelemetryEvent{
			Type:      apikit.EventTypeHTTPRequestSucceeded,
			SessionID: "abc",
		})
	}

	// BatchSize is a flush trigger threshold, not a hard slice cap: when
	// the buffer reaches the threshold the flusher drains everything
	// pending. So 6 events still produce a single batch — but at least
	// 5 records must be present, proving the size-driven wakeup fired.
	if !waitFor(t, 500*time.Millisecond, func() bool {
		return len(capt.snapshot()) >= 1
	}) {
		t.Fatal("size-driven flush did not happen")
	}
	snapshot := capt.snapshot()
	if len(snapshot) == 0 {
		t.Fatalf("expected at least 1 batch")
	}
	var got otlpLogsRequest
	if err := json.Unmarshal(snapshot[0], &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.ResourceLogs) == 0 || len(got.ResourceLogs[0].ScopeLogs) == 0 {
		t.Fatalf("malformed payload")
	}
	if n := len(got.ResourceLogs[0].ScopeLogs[0].LogRecords); n < 5 {
		t.Errorf("expected at least 5 records in flushed batch, got %d", n)
	}
}

func TestExporter_BatchesAndFlushesOnInterval(t *testing.T) {
	srv, capt := captureRequests(t, http.StatusOK)
	exp := makeExporter(t, srv, func(c *Config) {
		c.BatchSize = 1000 // never size-flush
		c.FlushInterval = 80 * time.Millisecond
	})

	exp.Record(apikit.TelemetryEvent{Type: apikit.EventTypeAnalytics})
	exp.Record(apikit.TelemetryEvent{Type: apikit.EventTypeAnalytics})

	if !waitFor(t, 500*time.Millisecond, func() bool { return len(capt.snapshot()) >= 1 }) {
		t.Fatal("interval-driven flush did not happen")
	}
}

func TestExporter_RetriesOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	exp := makeExporter(t, srv, func(c *Config) {
		c.BatchSize = 1
	})

	// Override the per-attempt sleep so we don't wait 1 second in tests.
	exp.cfg.RetryAttempts = 3

	exp.Record(apikit.TelemetryEvent{Type: apikit.EventTypeAnalytics})

	if !waitFor(t, 5*time.Second, func() bool { return calls.Load() >= 2 }) {
		t.Fatalf("expected at least 2 attempts, got %d", calls.Load())
	}
}

func TestExporter_DropsOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	var errMu sync.Mutex
	var errs []error
	exp := makeExporter(t, srv, func(c *Config) {
		c.BatchSize = 1
		c.ErrorHandler = func(err error) {
			errMu.Lock()
			errs = append(errs, err)
			errMu.Unlock()
		}
	})

	exp.Record(apikit.TelemetryEvent{Type: apikit.EventTypeAnalytics})

	if !waitFor(t, 1*time.Second, func() bool {
		errMu.Lock()
		defer errMu.Unlock()
		return len(errs) >= 1
	}) {
		t.Fatal("expected ErrorHandler to fire on 4xx")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected exactly 1 attempt for 4xx (no retry), got %d", got)
	}
}

func TestExporter_StopFlushesPending(t *testing.T) {
	srv, capt := captureRequests(t, http.StatusOK)
	exp := makeExporter(t, srv, func(c *Config) {
		c.BatchSize = 1000
		c.FlushInterval = 1 * time.Hour
	})

	exp.Record(apikit.TelemetryEvent{Type: apikit.EventTypeAnalytics})
	exp.Record(apikit.TelemetryEvent{Type: apikit.EventTypeAnalytics})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := exp.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := len(capt.snapshot()); got != 1 {
		t.Errorf("expected pending batch flushed on Stop, got %d batches", got)
	}
}

func TestExporter_RecordAfterStop_DoesNotPanic(t *testing.T) {
	// Documented contract (docs/review_rationale.md): late records are
	// dropped silently after Stop. This test pins that behavior so a
	// future "fail loud after Stop" change is a deliberate decision.
	srv, _ := captureRequests(t, http.StatusOK)
	exp := makeExporter(t, srv, func(c *Config) { c.BatchSize = 1000 })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := exp.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Record after Stop must not panic, even concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Record panicked after Stop: %v", r)
				}
			}()
			exp.Record(apikit.TelemetryEvent{Type: apikit.EventTypeAnalytics})
		}()
	}
	wg.Wait()
}

func TestExporter_ConcurrentStop_IsSafe(t *testing.T) {
	// Real-world Stop callers include defer chains AND signal
	// handlers — both can fire at once. Without sync.Once, two
	// concurrent Stops both close(stopReq) and panic.
	srv, _ := captureRequests(t, http.StatusOK)
	exp := makeExporter(t, srv)

	const goroutines = 5
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("concurrent Stop panicked: %v", r)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = exp.Stop(ctx)
		}()
	}
	wg.Wait()
}

func TestExporter_StopIsIdempotent(t *testing.T) {
	srv, _ := captureRequests(t, http.StatusOK)
	exp := makeExporter(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := exp.Stop(ctx); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := exp.Stop(ctx); err != nil {
		t.Errorf("second Stop must be a no-op, got %v", err)
	}
	if err := exp.Stop(ctx); err != nil {
		t.Errorf("third Stop must be a no-op, got %v", err)
	}
}

func TestExporter_StartIsIdempotent(t *testing.T) {
	// Multiple Start calls must not spawn duplicate flusher
	// goroutines (the once.Do guard does that work).
	srv, _ := captureRequests(t, http.StatusOK)
	cfg := Config{
		Endpoint:      srv.URL,
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
		HTTPClient:    srv.Client(),
	}
	exp, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := exp.Start(context.Background()); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := exp.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestExporter_RetryGivesUpAfterAllAttempts(t *testing.T) {
	// All 5xx forever: ErrorHandler must fire exactly once with the
	// last error, no infinite retry loop.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	var errMu sync.Mutex
	var errs []error
	cfg := Config{
		Endpoint:      srv.URL,
		BatchSize:     1,
		FlushInterval: 1 * time.Hour,
		HTTPClient:    srv.Client(),
		RetryAttempts: 2, // Two attempts total
		ErrorHandler: func(err error) {
			errMu.Lock()
			errs = append(errs, err)
			errMu.Unlock()
		},
	}
	exp, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := exp.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exp.Stop(ctx)
	})

	// Override delay-doubling to keep the test fast — each retry
	// would sleep 1s. We can't reach the private `delay` var, so
	// instead we just record one event and wait for the error
	// handler.
	exp.Record(apikit.TelemetryEvent{Type: apikit.EventTypeAnalytics})

	if !waitFor(t, 5*time.Second, func() bool {
		errMu.Lock()
		defer errMu.Unlock()
		return len(errs) >= 1
	}) {
		t.Fatal("expected ErrorHandler to fire after retries exhausted")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected exactly 2 attempts (RetryAttempts=2), got %d", got)
	}
}

func TestNew_RejectsEmptyEndpoint(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error on empty endpoint")
	}
}

func TestBuildLogsURL_AppendsPath(t *testing.T) {
	cases := []struct{ in, out string }{
		{"http://x", "http://x/v1/logs"},
		{"http://x/", "http://x/v1/logs"},
		{"http://x/v1/logs", "http://x/v1/logs"},
		{"http://x/v1/logs/", "http://x/v1/logs"},
	}
	for _, tc := range cases {
		if got := buildLogsURL(tc.in); got != tc.out {
			t.Errorf("buildLogsURL(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}
