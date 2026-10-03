package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"relay/internal/platform/logging"
)

// observedLogger records log entries in memory so tests can inspect them.
func observedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestChain_RunsMiddlewareOutermostFirst(t *testing.T) {
	var order []string
	mw := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "handler") }),
		mw("a"), mw("b"))

	serve(h, http.MethodGet, "/")

	if got := len(order); got != 3 || order[0] != "a" || order[1] != "b" || order[2] != "handler" {
		t.Fatalf("order = %v, want [a b handler]", order)
	}
}

func TestRecover_PanicBecomes500InOurShape(t *testing.T) {
	log, logs := observedLogger()
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }),
		Recover(log), RequestID(log))

	rec := serve(h, http.MethodGet, "/x")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
	body := decodeError(t, rec)
	if body.Code != CodeInternal {
		t.Errorf("error code = %q, want %q", body.Code, CodeInternal)
	}
	if body.RequestID == "" || body.RequestID != rec.Header().Get(RequestIDHeader) {
		t.Errorf("request_id %q should match header %q", body.RequestID, rec.Header().Get(RequestIDHeader))
	}
	if logs.FilterMessage("panic recovered").Len() != 1 {
		t.Errorf("panic was not logged; logs: %v", logs.All())
	}
}

func TestRecover_ServerKeepsWorkingAfterPanic(t *testing.T) {
	log, _ := observedLogger()
	calls := 0
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			panic("first request explodes")
		}
		w.WriteHeader(http.StatusOK)
	}), Recover(log))

	serve(h, http.MethodGet, "/")
	rec := serve(h, http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("second request code = %d, want 200", rec.Code)
	}
}

func TestRequestID_SetsUniqueHeaderAndIgnoresClientValue(t *testing.T) {
	log, _ := observedLogger()
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), RequestID(log))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "client-chosen")
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req)
	rec2 := serve(h, http.MethodGet, "/")

	id1, id2 := rec1.Header().Get(RequestIDHeader), rec2.Header().Get(RequestIDHeader)
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("want two different non-empty IDs, got %q and %q", id1, id2)
	}
	if id1 == "client-chosen" {
		t.Fatal("client-supplied request ID must be ignored")
	}
}

func TestRequestID_HandlerLoggerCarriesTheID(t *testing.T) {
	log, logs := observedLogger()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logging.FromContext(r.Context()).Info("inside handler")
	}), RequestID(log))

	rec := serve(h, http.MethodGet, "/")

	entries := logs.FilterMessage("inside handler").All()
	if len(entries) != 1 {
		t.Fatalf("want 1 handler log line, got %d", len(entries))
	}
	if got := entries[0].ContextMap()["request_id"]; got != rec.Header().Get(RequestIDHeader) {
		t.Fatalf("request_id in log = %v, header = %q", got, rec.Header().Get(RequestIDHeader))
	}
}

func TestRequestLog_RecordsMethodPathStatusAndDuration(t *testing.T) {
	log, logs := observedLogger()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}), RequestID(log), RequestLog())

	serve(h, http.MethodPost, "/api/notes")

	entries := logs.FilterMessage("request").All()
	if len(entries) != 1 {
		t.Fatalf("want 1 request log line, got %d: %v", len(entries), logs.All())
	}
	f := entries[0].ContextMap()
	if f["method"] != "POST" || f["path"] != "/api/notes" || f["status"] != int64(http.StatusTeapot) {
		t.Fatalf("unexpected fields: %v", f)
	}
	if _, ok := f["duration_ms"]; !ok {
		t.Fatalf("duration_ms missing: %v", f)
	}
	if f["request_id"] == nil {
		t.Fatalf("request_id missing: %v", f)
	}
}

func TestRequestLog_HandlerWritesNoStatus_LogsImplicit200(t *testing.T) {
	log, logs := observedLogger()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok")) // no explicit WriteHeader → Go sends 200
	}), RequestID(log), RequestLog())

	serve(h, http.MethodGet, "/")

	if got := logs.FilterMessage("request").All()[0].ContextMap()["status"]; got != int64(200) {
		t.Fatalf("status = %v, want 200", got)
	}
}
