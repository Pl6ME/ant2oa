package main

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goccy/go-json"
)

// ================= Metrics =================

// Metrics holds application metrics
type Metrics struct {
	TotalRequests     atomic.Int64
	SuccessRequests   atomic.Int64
	ErrorRequests     atomic.Int64
	TotalLatencyMs    atomic.Int64
	UpstreamErrors    atomic.Int64
	RateLimitedCount  atomic.Int64
	ActiveConnections atomic.Int64
	StartTime         time.Time

	// Per-endpoint metrics
	endpointMetrics sync.Map // map[string]*EndpointMetrics
}

// EndpointMetrics holds per-endpoint statistics
type EndpointMetrics struct {
	Requests  atomic.Int64
	Errors    atomic.Int64
	LatencyMs atomic.Int64
}

var metrics = &Metrics{
	StartTime: time.Now(),
}

// RecordRequest records a request metric
func (m *Metrics) RecordRequest(endpoint string, latencyMs int64, isError bool) {
	m.TotalRequests.Add(1)
	m.TotalLatencyMs.Add(latencyMs)

	if isError {
		m.ErrorRequests.Add(1)
	} else {
		m.SuccessRequests.Add(1)
	}

	// Per-endpoint
	val, _ := m.endpointMetrics.LoadOrStore(endpoint, &EndpointMetrics{})
	em := val.(*EndpointMetrics)
	em.Requests.Add(1)
	em.LatencyMs.Add(latencyMs)
	if isError {
		em.Errors.Add(1)
	}
}

// metricsHandler returns metrics in Prometheus-compatible format
func metricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(metrics.StartTime).Seconds()
		totalReqs := metrics.TotalRequests.Load()
		avgLatency := float64(0)
		if totalReqs > 0 {
			avgLatency = float64(metrics.TotalLatencyMs.Load()) / float64(totalReqs)
		}

		// Prometheus text format
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		output := "# HELP ant2oa_uptime_seconds Time since server start\n"
		output += "# TYPE ant2oa_uptime_seconds gauge\n"
		output += "ant2oa_uptime_seconds " + formatFloat(uptime) + "\n\n"

		output += "# HELP ant2oa_requests_total Total number of requests\n"
		output += "# TYPE ant2oa_requests_total counter\n"
		output += "ant2oa_requests_total " + formatInt(totalReqs) + "\n\n"

		output += "# HELP ant2oa_requests_success_total Successful requests\n"
		output += "# TYPE ant2oa_requests_success_total counter\n"
		output += "ant2oa_requests_success_total " + formatInt(metrics.SuccessRequests.Load()) + "\n\n"

		output += "# HELP ant2oa_requests_error_total Failed requests\n"
		output += "# TYPE ant2oa_requests_error_total counter\n"
		output += "ant2oa_requests_error_total " + formatInt(metrics.ErrorRequests.Load()) + "\n\n"

		output += "# HELP ant2oa_upstream_errors_total Upstream errors\n"
		output += "# TYPE ant2oa_upstream_errors_total counter\n"
		output += "ant2oa_upstream_errors_total " + formatInt(metrics.UpstreamErrors.Load()) + "\n\n"

		output += "# HELP ant2oa_rate_limited_total Rate limited requests\n"
		output += "# TYPE ant2oa_rate_limited_total counter\n"
		output += "ant2oa_rate_limited_total " + formatInt(metrics.RateLimitedCount.Load()) + "\n\n"

		output += "# HELP ant2oa_active_connections Current active connections\n"
		output += "# TYPE ant2oa_active_connections gauge\n"
		output += "ant2oa_active_connections " + formatInt(metrics.ActiveConnections.Load()) + "\n\n"

		output += "# HELP ant2oa_avg_latency_ms Average request latency in milliseconds\n"
		output += "# TYPE ant2oa_avg_latency_ms gauge\n"
		output += "ant2oa_avg_latency_ms " + formatFloat(avgLatency) + "\n"

		w.Write([]byte(output))
	}
}

// metricsJSONHandler returns metrics as JSON
func metricsJSONHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		totalReqs := metrics.TotalRequests.Load()
		avgLatency := float64(0)
		if totalReqs > 0 {
			avgLatency = float64(metrics.TotalLatencyMs.Load()) / float64(totalReqs)
		}

		data := map[string]any{
			"uptime_seconds":     time.Since(metrics.StartTime).Seconds(),
			"total_requests":     totalReqs,
			"success_requests":   metrics.SuccessRequests.Load(),
			"error_requests":     metrics.ErrorRequests.Load(),
			"upstream_errors":    metrics.UpstreamErrors.Load(),
			"rate_limited":       metrics.RateLimitedCount.Load(),
			"active_connections": metrics.ActiveConnections.Load(),
			"avg_latency_ms":     avgLatency,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}
}

func formatFloat(f float64) string {
	return json.Number(json.RawMessage(floatToString(f))).String()
}

func formatInt(i int64) string {
	return json.Number(json.RawMessage(intToString(i))).String()
}

func floatToString(f float64) string {
	return string(json.RawMessage([]byte(func() []byte {
		b, _ := json.Marshal(f)
		return b
	}())))
}

func intToString(i int64) string {
	return string(json.RawMessage([]byte(func() []byte {
		b, _ := json.Marshal(i)
		return b
	}())))
}
