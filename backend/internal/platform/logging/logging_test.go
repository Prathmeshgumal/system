package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestNew_InfoLevel_HidesDebug(t *testing.T) {
	var buf bytes.Buffer
	log, err := newWithWriter("info", "json", &buf)
	if err != nil {
		t.Fatalf("newWithWriter() error = %v", err)
	}

	log.Debug("hidden")
	log.Info("shown")
	_ = log.Sync()

	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Errorf("debug line printed at info level:\n%s", out)
	}
	if !strings.Contains(out, "shown") {
		t.Errorf("info line missing:\n%s", out)
	}
}

func TestNew_JSONFormat_EmitsOneJSONObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	log, err := newWithWriter("debug", "json", &buf)
	if err != nil {
		t.Fatalf("newWithWriter() error = %v", err)
	}

	log.Info("login failed", zap.Int("user_id", 42))
	_ = log.Sync()

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if line["msg"] != "login failed" || line["user_id"] != float64(42) || line["level"] != "info" {
		t.Fatalf("unexpected fields: %v", line)
	}
}

func TestNew_ConsoleFormat_IsNotJSON(t *testing.T) {
	var buf bytes.Buffer
	log, err := newWithWriter("debug", "console", &buf)
	if err != nil {
		t.Fatalf("newWithWriter() error = %v", err)
	}

	log.Info("hello", zap.String("k", "v"))
	_ = log.Sync()

	if json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("console format produced JSON: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("message missing: %s", buf.String())
	}
}

func TestNew_UnknownLevel_ReturnsError(t *testing.T) {
	if _, err := newWithWriter("loud", "json", &bytes.Buffer{}); err == nil {
		t.Fatal("want error for unknown level")
	}
}

func TestFromContext_ReturnsLoggerStoredByWithLogger(t *testing.T) {
	var buf bytes.Buffer
	log, _ := newWithWriter("info", "json", &buf)

	ctx := WithLogger(context.Background(), log.With(zap.String("request_id", "abc")))
	FromContext(ctx).Info("from handler")
	_ = log.Sync()

	if !strings.Contains(buf.String(), `"request_id":"abc"`) {
		t.Fatalf("request_id not carried through the context:\n%s", buf.String())
	}
}

func TestFromContext_NoLogger_ReturnsUsableFallback(t *testing.T) {
	l := FromContext(context.Background())
	if l == nil {
		t.Fatal("FromContext returned nil")
	}
	l.Info("must not panic")
}
