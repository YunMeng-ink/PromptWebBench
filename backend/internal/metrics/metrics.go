// Package metrics 提供 Prometheus 指标采集。指标端点仅回环可达（G-02）。
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 持有独立注册表，避免测试重复注册时的全局冲突。
type Metrics struct {
	registry *prometheus.Registry

	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
}

func New() *Metrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{
		registry: registry,
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "prompt_benchmark",
			Name:      "http_requests_total",
			Help:      "HTTP 请求总数，按方法、路由与状态码分组。",
		}, []string{"method", "route", "status"}),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "prompt_benchmark",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP 请求耗时分布。",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	registry.MustRegister(m.HTTPRequests, m.HTTPDuration)
	return m
}

// Handler 返回指标抓取端点。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
