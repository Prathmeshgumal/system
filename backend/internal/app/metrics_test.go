package app_test

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"relay/internal/metrics"
)

func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	b, _ := io.ReadAll(rec.Body)
	return string(b)
}

func TestMetrics_RoutePatternsNotRawPaths(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	ws := bob.connect()
	alice.send(c.ID, "hi")
	next(t, ws) // delivered
	alice.do("GET", fmt.Sprintf("/api/chats/%d/messages", c.ID), nil, nil)
	alice.do("GET", "/api/definitely-not-a-route", nil, nil)

	out := scrape(t)
	for _, want := range []string{
		`http_requests_total{code="200",method="GET",route="/api/chats/{id}/messages"}`,
		`http_requests_total{code="201",method="POST",route="/api/chats/{id}/messages"}`,
		`http_requests_total{code="404",method="GET",route="unmatched"}`,
		`chat_messages_sent_total`,
		`ws_events_sent_total{type="message.created"}`,
		`go_goroutines`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(out, fmt.Sprintf(`route="/api/chats/%d`, c.ID)) {
		t.Error("a raw path with an id leaked into a label: one time series per chat")
	}
}
