package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type metricKey struct {
	method string
	route  string
	status int
}

type metricValue struct {
	count           uint64
	durationSeconds float64
}

type Metrics struct {
	mu          sync.Mutex
	http        map[metricKey]metricValue
	rateLimited map[string]uint64
}

func NewMetrics() *Metrics {
	return &Metrics{http: make(map[metricKey]metricValue), rateLimited: make(map[string]uint64)}
}

func (m *Metrics) Observe(method, route string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := metricKey{method: method, route: route, status: status}
	value := m.http[key]
	value.count++
	value.durationSeconds += duration.Seconds()
	m.http[key] = value
}

func (m *Metrics) IncrementRateLimited(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimited[name]++
}

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(m.render()))
	})
}

func (m *Metrics) render() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]metricKey, 0, len(m.http))
	for key := range m.http {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := keys[i], keys[j]
		if left.route != right.route {
			return left.route < right.route
		}
		if left.method != right.method {
			return left.method < right.method
		}
		return left.status < right.status
	})
	var output strings.Builder
	output.WriteString("# HELP zeexco_http_requests_total Total HTTP requests.\n")
	output.WriteString("# TYPE zeexco_http_requests_total counter\n")
	for _, key := range keys {
		value := m.http[key]
		labels := fmt.Sprintf("method=%q,route=%q,status=%q", key.method, key.route, strconv.Itoa(key.status))
		fmt.Fprintf(&output, "zeexco_http_requests_total{%s} %d\n", labels, value.count)
		fmt.Fprintf(&output, "zeexco_http_request_duration_seconds_sum{%s} %g\n", labels, value.durationSeconds)
		fmt.Fprintf(&output, "zeexco_http_request_duration_seconds_count{%s} %d\n", labels, value.count)
	}
	output.WriteString("# HELP zeexco_rate_limited_requests_total Requests rejected by a local rate limiter.\n")
	output.WriteString("# TYPE zeexco_rate_limited_requests_total counter\n")
	names := make([]string, 0, len(m.rateLimited))
	for name := range m.rateLimited {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&output, "zeexco_rate_limited_requests_total{limiter=%q} %d\n", name, m.rateLimited[name])
	}
	return output.String()
}
