// Copyright (c) 2026 Idriss ELIGUENE
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package observability contains bounded operational logging and metrics. It
// never decides authorization or product state; those remain authoritative in
// the application and Kubernetes objects.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	RequestIDHeader = "X-Request-ID"
	DefaultLogLevel = "INFO"
)

type requestStatsKey struct{}

// RequestStats records bounded, request-local read instrumentation. It is
// deliberately carried in context rather than a process-global accumulator so
// one authenticated request cannot be confused with another identity or
// namespace.
type RequestStats struct {
	mu sync.Mutex

	AuthorizationCalls    int
	AuthorizationDuration time.Duration
	KubernetesCalls       int
	KubernetesDuration    time.Duration
	DiscoveryCalls        int
	DiscoveryDuration     time.Duration
	ProjectionDuration    time.Duration
}

func WithRequestStats(ctx context.Context, stats *RequestStats) context.Context {
	return context.WithValue(ctx, requestStatsKey{}, stats)
}

func RequestStatsFromContext(ctx context.Context) *RequestStats {
	if ctx == nil {
		return nil
	}
	stats, _ := ctx.Value(requestStatsKey{}).(*RequestStats)
	return stats
}

func (s *RequestStats) AddAuthorization(d time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.AuthorizationCalls++
	s.AuthorizationDuration += d
	s.mu.Unlock()
}

func (s *RequestStats) AddKubernetes(d time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.KubernetesCalls++
	s.KubernetesDuration += d
	s.mu.Unlock()
}

func (s *RequestStats) AddDiscovery(d time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.DiscoveryCalls++
	s.DiscoveryDuration += d
	s.mu.Unlock()
}

func (s *RequestStats) SetProjectionDuration(d time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.ProjectionDuration = d
	s.mu.Unlock()
}

type RequestStatsSnapshot struct {
	AuthorizationCalls    int
	AuthorizationDuration time.Duration
	KubernetesCalls       int
	KubernetesDuration    time.Duration
	DiscoveryCalls        int
	DiscoveryDuration     time.Duration
	ProjectionDuration    time.Duration
}

func (s *RequestStats) Snapshot() RequestStatsSnapshot {
	if s == nil {
		return RequestStatsSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return RequestStatsSnapshot{
		AuthorizationCalls:    s.AuthorizationCalls,
		AuthorizationDuration: s.AuthorizationDuration,
		KubernetesCalls:       s.KubernetesCalls,
		KubernetesDuration:    s.KubernetesDuration,
		DiscoveryCalls:        s.DiscoveryCalls,
		DiscoveryDuration:     s.DiscoveryDuration,
		ProjectionDuration:    s.ProjectionDuration,
	}
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func ParseLevel(value string) (Level, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "INFO":
		return LevelInfo, nil
	case "DEBUG":
		return LevelDebug, nil
	case "WARN", "WARNING":
		return LevelWarn, nil
	case "ERROR":
		return LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}

type Logger struct {
	mu    sync.Mutex
	out   *log.Logger
	level Level
}

func NewLogger(out io.Writer, level string) (*Logger, error) {
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = io.Discard
	}
	return &Logger{out: log.New(out, "", 0), level: parsed}, nil
}

func (l *Logger) Event(level Level, event string, fields map[string]interface{}) {
	if l == nil || level < l.level || strings.TrimSpace(event) == "" {
		return
	}
	record := map[string]interface{}{"level": levelName(level), "event": event, "time": time.Now().UTC().Format(time.RFC3339Nano)}
	for key, value := range fields {
		record[key] = sanitizeField(key, value)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	l.mu.Lock()
	l.out.Print(string(data))
	l.mu.Unlock()
}

func (l *Logger) Debug(event string, fields map[string]interface{}) {
	l.Event(LevelDebug, event, fields)
}
func (l *Logger) Info(event string, fields map[string]interface{}) { l.Event(LevelInfo, event, fields) }
func (l *Logger) Warn(event string, fields map[string]interface{}) { l.Event(LevelWarn, event, fields) }
func (l *Logger) Error(event string, fields map[string]interface{}) {
	l.Event(LevelError, event, fields)
}

func levelName(level Level) string {
	switch level {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func sanitizeField(key string, value interface{}) interface{} {
	lower := strings.ToLower(key)
	for _, forbidden := range []string{"secret", "signature", "authorization", "token", "kubeconfig", "credential", "password"} {
		if strings.Contains(lower, forbidden) {
			return "[REDACTED]"
		}
	}
	switch typed := value.(type) {
	case error:
		return boundedError(typed)
	case string:
		if len(typed) > 512 {
			return typed[:512] + "…"
		}
	}
	return value
}

func boundedError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 512 {
		message = message[:512] + "…"
	}
	return message
}

func RequestID(header string) string {
	if requestIDPattern.MatchString(strings.TrimSpace(header)) {
		return strings.TrimSpace(header)
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(bytes[:])
}

func Route(path string) string {
	if path == "/" || path == "/workbench.js" || path == "/healthz/startup" || path == "/healthz/live" || path == "/healthz/ready" {
		return path
	}
	for _, prefix := range []string{"/api/v08/", "/api/governance/", "/api/observations/", "/api/proposals/"} {
		if strings.HasPrefix(path, prefix) {
			return prefix + ":id"
		}
	}
	if path == "/api/observations" || path == "/api/proposals" || path == "/api/workloads" || path == "/api/projection" {
		return path
	}
	return "/other"
}

type ResponseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *ResponseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
func (r *ResponseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}
func (r *ResponseRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

type Metrics struct {
	mu       sync.Mutex
	counters map[string]metricValue
	gauges   map[string]metricValue
	active   int64
}

func (m *Metrics) ActiveRequests(delta int64) int64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active += delta
	if m.active < 0 {
		m.active = 0
	}
	return m.active
}

type metricValue struct {
	name   string
	labels []label
	value  float64
}

type label struct{ name, value string }

func NewMetrics() *Metrics {
	return &Metrics{counters: make(map[string]metricValue), gauges: make(map[string]metricValue)}
}

func (m *Metrics) add(target map[string]metricValue, name string, value float64, labels map[string]string) {
	if m == nil || name == "" {
		return
	}
	ordered := make([]label, 0, len(labels))
	for key, val := range labels {
		ordered = append(ordered, label{name: key, value: val})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].name < ordered[j].name })
	key := name
	for _, item := range ordered {
		key += "\x00" + item.name + "=" + item.value
	}
	m.mu.Lock()
	item := target[key]
	item.name, item.labels, item.value = name, ordered, item.value+value
	target[key] = item
	m.mu.Unlock()
}

func (m *Metrics) Inc(name string, labels map[string]string) { m.add(m.counters, name, 1, labels) }
func (m *Metrics) Set(name string, value float64, labels map[string]string) {
	if m == nil {
		return
	}
	ordered := make([]label, 0, len(labels))
	for key, val := range labels {
		ordered = append(ordered, label{name: key, value: val})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].name < ordered[j].name })
	key := name
	for _, item := range ordered {
		key += "\x00" + item.name + "=" + item.value
	}
	m.mu.Lock()
	m.gauges[key] = metricValue{name: name, labels: ordered, value: value}
	m.mu.Unlock()
}

func (m *Metrics) HTTP(method, route string, status int) {
	class := strconv.Itoa(status/100) + "xx"
	m.Inc("operations_center_http_requests_total", map[string]string{"method": method, "route": route, "status_class": class})
}
func (m *Metrics) HTTPDuration(seconds float64, method, route string) {
	if seconds < 0 {
		seconds = 0
	}
	labels := map[string]string{"method": method, "route": route}
	m.add(m.counters, "operations_center_http_request_duration_seconds_sum", seconds, labels)
	m.add(m.counters, "operations_center_http_request_duration_seconds_count", 1, labels)
}
func (m *Metrics) AuthFailure(reason string) {
	m.Inc("operations_center_authentication_failures_total", map[string]string{"reason": boundedReason(reason)})
}
func (m *Metrics) AuthorizationDenied(reason string) {
	m.Inc("operations_center_authorization_denials_total", map[string]string{"reason": boundedReason(reason)})
}
func (m *Metrics) Governance(operation, result, reason string) {
	m.Inc("operations_center_governance_operations_total", map[string]string{"operation": operation, "result": boundedReason(result), "reason": boundedReason(reason)})
}
func (m *Metrics) ProjectionExcluded() {
	m.Inc("operations_center_projection_excluded_objects_total", nil)
}
func (m *Metrics) ProjectionExcludedCount(count int) {
	if count <= 0 {
		return
	}
	m.add(m.counters, "operations_center_projection_excluded_objects_total", float64(count), nil)
}
func (m *Metrics) ExecutorClaim(result string) {
	m.Inc("observation_executor_claims_total", map[string]string{"result": boundedReason(result)})
}
func (m *Metrics) ExecutorClaimConflict()       { m.Inc("observation_executor_claim_conflicts_total", nil) }
func (m *Metrics) ExecutorActive(value float64) { m.Set("observation_executor_active", value, nil) }
func (m *Metrics) ExecutorCompleted()           { m.Inc("observation_executor_completed_total", nil) }
func (m *Metrics) ExecutorFailed(reason string) {
	m.Inc("observation_executor_failed_total", map[string]string{"reason": boundedReason(reason)})
}
func (m *Metrics) ExecutorLost() { m.Inc("observation_executor_executor_lost_total", nil) }
func (m *Metrics) LeaseRenewalFailure() {
	m.Inc("observation_executor_lease_renewal_failures_total", nil)
}
func (m *Metrics) GadgetFailure(reason string) {
	m.Inc("observation_executor_gadget_failures_total", map[string]string{"reason": boundedReason(reason)})
}

func boundedReason(reason string) string {
	value := strings.ToUpper(strings.TrimSpace(reason))
	if len(value) > 64 || value == "" {
		return "UNKNOWN"
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return "UNKNOWN"
		}
	}
	return value
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.counters)+len(m.gauges))
	values := make(map[string]metricValue, len(m.counters)+len(m.gauges))
	for key, value := range m.counters {
		keys = append(keys, key)
		values[key] = value
	}
	for key, value := range m.gauges {
		keys = append(keys, key)
		values[key] = value
	}
	sort.Strings(keys)
	seen := make(map[string]bool)
	for _, key := range keys {
		item := values[key]
		if !seen[item.name] {
			kind := "counter"
			if _, ok := m.gauges[key]; ok {
				kind = "gauge"
			}
			_, _ = fmt.Fprintf(w, "# TYPE %s %s\n", item.name, kind)
			seen[item.name] = true
		}
		_, _ = fmt.Fprintf(w, "%s%s %g\n", item.name, formatLabels(item.labels), item.value)
	}
}

func formatLabels(labels []label) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for _, item := range labels {
		parts = append(parts, fmt.Sprintf("%s=%q", item.name, item.value))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func IsMetricsPath(path string) bool { return path == "/metrics" }
