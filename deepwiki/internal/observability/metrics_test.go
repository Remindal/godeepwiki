package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetricsHandler(t *testing.T) {
	m := Register() // 注册全局指标（只允许调用一次；本测试进程内仅此处调用）。
	m.RabbitMQQueueDepth.WithLabelValues("deepwiki.task.jobs").Set(0)
	m.RedisOpDuration.WithLabelValues("publish").Observe(0.001)

	req, _ := http.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("unexpected content type: %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "deepwiki_worker_busy") {
		t.Fatal("missing deepwiki_worker_busy metric")
	}
	if !strings.Contains(body, "deepwiki_rabbitmq_queue_depth") {
		t.Fatal("missing deepwiki_rabbitmq_queue_depth metric")
	}
}
