// Package otlp implements an OTLP/HTTP exporter for apikit.TelemetryEvent
// using the JSON wire format. JSON OTLP is the same schema as protobuf
// OTLP — every collector that speaks OTLP/HTTP accepts both — but it lets
// us avoid pulling in the opentelemetry-proto module just to ship logs.
package otlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SocialGouv/claw-code-go/internal/apikit"
)

// Defaults match the OTLP-collector convention plus modest values that
// keep memory bounded without flushing too aggressively.
const (
	DefaultBatchSize     = 512
	DefaultFlushInterval = 5 * time.Second
	defaultRetryAttempts = 3
)

// OTLP SeverityNumber values per the OpenTelemetry logs data model.
// We only emit two levels — INFO for everything except outright
// failures, ERROR for HTTP-failed events.
const (
	severityNumberInfo  = 9
	severityNumberError = 17
)

// Config configures the exporter. Endpoint is the only required field.
type Config struct {
	// Endpoint is the OTLP/HTTP receiver URL. The exporter appends
	// /v1/logs unless the endpoint already contains a path component.
	// Standard env var: OTEL_EXPORTER_OTLP_ENDPOINT.
	Endpoint string

	// Headers are appended to every request. Useful for auth tokens.
	// Standard env var: OTEL_EXPORTER_OTLP_HEADERS (comma-separated key=value).
	Headers map[string]string

	// ServiceName populates the resource service.name attribute.
	// Defaults to "claw-code-go".
	ServiceName string

	// BatchSize triggers a flush once the in-memory buffer reaches this
	// many events. Defaults to DefaultBatchSize.
	BatchSize int

	// FlushInterval triggers a flush at most this often regardless of
	// fill level. Defaults to DefaultFlushInterval.
	FlushInterval time.Duration

	// HTTPClient overrides the default. Tests inject httptest clients here.
	HTTPClient *http.Client

	// RetryAttempts caps retries on transient (5xx) failures. The
	// exporter does NOT retry 4xx — those are programmer/auth errors and
	// retrying just wastes batches.
	RetryAttempts int

	// ErrorHandler observes export failures so operators can wire alerts.
	// If nil, errors are silently dropped (matching JsonlTelemetrySink).
	ErrorHandler func(error)
}

// Exporter is an apikit.TelemetrySink that batches events and ships them
// to an OTLP/HTTP collector. Stop must be called for clean shutdown — it
// drains the buffer and waits for the background flusher.
type Exporter struct {
	cfg     Config
	url     string
	client  *http.Client
	headers map[string]string

	mu     sync.Mutex
	buffer []apikit.TelemetryEvent

	flushReq chan struct{}
	stopReq  chan struct{}
	stopped  chan struct{}
	once     sync.Once
	stopOnce sync.Once
}

// New constructs an exporter. It does not start the flusher goroutine —
// callers must invoke Start before recording events to enable
// time-based flushes.
func New(cfg Config) (*Exporter, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("otlp: Endpoint is required")
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultFlushInterval
	}
	if cfg.RetryAttempts <= 0 {
		cfg.RetryAttempts = defaultRetryAttempts
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		cfg.ServiceName = "claw-code-go"
	}

	url := buildLogsURL(cfg.Endpoint)

	headers := make(map[string]string, len(cfg.Headers)+2)
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	headers["Content-Type"] = "application/json"
	headers["User-Agent"] = "claw-code-go-otlp/1"

	return &Exporter{
		cfg:      cfg,
		url:      url,
		client:   cfg.HTTPClient,
		headers:  headers,
		buffer:   make([]apikit.TelemetryEvent, 0, cfg.BatchSize),
		flushReq: make(chan struct{}, 1),
		stopReq:  make(chan struct{}),
		stopped:  make(chan struct{}),
	}, nil
}

// Start launches the background flusher goroutine. Safe to call once.
// Subsequent calls are no-ops.
func (e *Exporter) Start(ctx context.Context) error {
	e.once.Do(func() {
		go e.runFlusher(ctx)
	})
	return nil
}

// Record buffers an event and signals a flush if the batch threshold is
// hit. It is safe to call concurrently and never blocks longer than the
// duration of a buffer-append.
func (e *Exporter) Record(event apikit.TelemetryEvent) {
	e.mu.Lock()
	e.buffer = append(e.buffer, event)
	full := len(e.buffer) >= e.cfg.BatchSize
	e.mu.Unlock()

	if full {
		// Non-blocking signal: the flusher coalesces multiple full
		// batches into one wakeup.
		select {
		case e.flushReq <- struct{}{}:
		default:
		}
	}
}

// Stop drains pending events and waits for the flusher to exit. It is
// idempotent and safe under concurrent calls — stopReq is closed
// exactly once even when multiple goroutines race to Stop.
func (e *Exporter) Stop(ctx context.Context) error {
	e.stopOnce.Do(func() { close(e.stopReq) })
	select {
	case <-e.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Exporter) runFlusher(ctx context.Context) {
	defer close(e.stopped)
	ticker := time.NewTicker(e.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.flush(context.Background())
			return
		case <-e.stopReq:
			e.flush(context.Background())
			return
		case <-ticker.C:
			e.flush(ctx)
		case <-e.flushReq:
			e.flush(ctx)
		}
	}
}

func (e *Exporter) flush(ctx context.Context) {
	e.mu.Lock()
	if len(e.buffer) == 0 {
		e.mu.Unlock()
		return
	}
	batch := e.buffer
	e.buffer = make([]apikit.TelemetryEvent, 0, e.cfg.BatchSize)
	e.mu.Unlock()

	if err := e.export(ctx, batch); err != nil && e.cfg.ErrorHandler != nil {
		e.cfg.ErrorHandler(err)
	}
}

// export ships a single batch and retries on 5xx with exponential
// backoff (1s, 2s, 4s). 4xx is a programmer error — no retry.
func (e *Exporter) export(ctx context.Context, batch []apikit.TelemetryEvent) error {
	body, err := encodeBatch(batch, e.cfg.ServiceName)
	if err != nil {
		return fmt.Errorf("otlp: encode: %w", err)
	}

	var lastErr error
	delay := time.Second
	for attempt := 0; attempt < e.cfg.RetryAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("otlp: build request: %w", err)
		}
		for k, v := range e.headers {
			req.Header.Set(k, v)
		}
		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			switch {
			case resp.StatusCode >= 200 && resp.StatusCode < 300:
				return nil
			case resp.StatusCode >= 400 && resp.StatusCode < 500:
				return fmt.Errorf("otlp: %s rejected batch (%d): %s", e.url, resp.StatusCode, string(respBody))
			default:
				lastErr = fmt.Errorf("otlp: %s returned %d: %s", e.url, resp.StatusCode, string(respBody))
			}
		}

		if attempt+1 < e.cfg.RetryAttempts {
			select {
			case <-time.After(delay):
				delay *= 2
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return lastErr
}

// ----- OTLP/HTTP JSON encoding ---------------------------------------------

// encodeBatch produces an OTLP/HTTP JSON payload (LogsService.Export
// request shape) for a batch of telemetry events. Each event becomes one
// LogRecord; the whole batch shares one ResourceLogs/ScopeLogs envelope.
func encodeBatch(batch []apikit.TelemetryEvent, serviceName string) ([]byte, error) {
	records := make([]otlpLogRecord, 0, len(batch))
	for _, ev := range batch {
		records = append(records, eventToLogRecord(ev))
	}
	payload := otlpLogsRequest{
		ResourceLogs: []otlpResourceLogs{{
			Resource: otlpResource{
				Attributes: []otlpKeyValue{stringAttr("service.name", serviceName)},
			},
			ScopeLogs: []otlpScopeLogs{{
				Scope:      otlpScope{Name: "claw-code-go"},
				LogRecords: records,
			}},
		}},
	}
	return json.Marshal(payload)
}

func eventToLogRecord(ev apikit.TelemetryEvent) otlpLogRecord {
	body, _ := json.Marshal(ev)
	rec := otlpLogRecord{
		TimeUnixNano:   strconv.FormatInt(time.Now().UnixNano(), 10),
		SeverityText:   severityFor(ev),
		SeverityNumber: severityNumberFor(ev),
		Body:           otlpAnyValue{StringValue: ptr(string(body))},
	}
	rec.Attributes = append(rec.Attributes, stringAttr("event.type", string(ev.Type)))
	if ev.SessionID != "" {
		rec.Attributes = append(rec.Attributes, stringAttr("session.id", ev.SessionID))
	}
	if ev.Method != "" {
		rec.Attributes = append(rec.Attributes, stringAttr("http.method", ev.Method))
	}
	if ev.Path != "" {
		rec.Attributes = append(rec.Attributes, stringAttr("http.path", ev.Path))
	}
	if ev.Status != 0 {
		rec.Attributes = append(rec.Attributes, intAttr("http.status_code", int64(ev.Status)))
	}
	if ev.RequestID != "" {
		rec.Attributes = append(rec.Attributes, stringAttr("http.request_id", ev.RequestID))
	}
	if ev.Error != "" {
		rec.Attributes = append(rec.Attributes, stringAttr("error.message", ev.Error))
		rec.Attributes = append(rec.Attributes, boolAttr("error.retryable", ev.Retryable))
	}
	return rec
}

func severityFor(ev apikit.TelemetryEvent) string {
	switch ev.Type {
	case apikit.EventTypeHTTPRequestFailed:
		return "ERROR"
	case apikit.EventTypeHTTPRequestSucceeded, apikit.EventTypeHTTPRequestStarted:
		return "INFO"
	case apikit.EventTypeAnalytics, apikit.EventTypeSessionTrace:
		return "INFO"
	}
	return "INFO"
}

func severityNumberFor(ev apikit.TelemetryEvent) int {
	if ev.Type == apikit.EventTypeHTTPRequestFailed {
		return severityNumberError
	}
	return severityNumberInfo
}

func buildLogsURL(endpoint string) string {
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(endpoint, "/v1/logs") {
		return endpoint
	}
	return endpoint + "/v1/logs"
}

func stringAttr(key, value string) otlpKeyValue {
	return otlpKeyValue{Key: key, Value: otlpAnyValue{StringValue: ptr(value)}}
}

func intAttr(key string, value int64) otlpKeyValue {
	return otlpKeyValue{Key: key, Value: otlpAnyValue{IntValue: ptr(strconv.FormatInt(value, 10))}}
}

func boolAttr(key string, value bool) otlpKeyValue {
	return otlpKeyValue{Key: key, Value: otlpAnyValue{BoolValue: &value}}
}

func ptr[T any](v T) *T { return &v }

type otlpLogsRequest struct {
	ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
}

type otlpResourceLogs struct {
	Resource  otlpResource    `json:"resource"`
	ScopeLogs []otlpScopeLogs `json:"scopeLogs"`
}

type otlpResource struct {
	Attributes []otlpKeyValue `json:"attributes,omitempty"`
}

type otlpScopeLogs struct {
	Scope      otlpScope       `json:"scope"`
	LogRecords []otlpLogRecord `json:"logRecords"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type otlpLogRecord struct {
	TimeUnixNano   string         `json:"timeUnixNano"`
	SeverityText   string         `json:"severityText,omitempty"`
	SeverityNumber int            `json:"severityNumber,omitempty"`
	Body           otlpAnyValue   `json:"body"`
	Attributes     []otlpKeyValue `json:"attributes,omitempty"`
}

type otlpKeyValue struct {
	Key   string       `json:"key"`
	Value otlpAnyValue `json:"value"`
}

// otlpAnyValue mirrors the protobuf AnyValue oneof shape: only one field
// is non-nil per value (stringValue / intValue / boolValue / etc).
// IntValue is a string in JSON OTLP because proto encodes int64 as
// string to preserve precision in JavaScript clients.
type otlpAnyValue struct {
	StringValue *string `json:"stringValue,omitempty"`
	IntValue    *string `json:"intValue,omitempty"`
	BoolValue   *bool   `json:"boolValue,omitempty"`
}
