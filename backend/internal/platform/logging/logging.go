// Package logging sets up the zap logger and carries a per-request logger
// through context.Context. See knowledge-base/notes/phase-0-notes.md, note 6.
package logging

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds the app logger, writing to stdout.
//
//	format "json"    → one JSON object per line (production, log collectors)
//	format "console" → human-readable, coloured levels (development)
func New(level, format string) (*zap.Logger, error) {
	return newWithWriter(level, format, os.Stdout)
}

func newWithWriter(level, format string, w io.Writer) (*zap.Logger, error) {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("log level: %w", err)
	}

	var enc zapcore.Encoder
	switch format {
	case "json":
		cfg := zap.NewProductionEncoderConfig()
		cfg.TimeKey = "time"
		cfg.EncodeTime = zapcore.ISO8601TimeEncoder
		enc = zapcore.NewJSONEncoder(cfg)
	case "console":
		cfg := zap.NewDevelopmentEncoderConfig()
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
		enc = zapcore.NewConsoleEncoder(cfg)
	default:
		return nil, fmt.Errorf("log format %q: want json or console", format)
	}

	core := zapcore.NewCore(enc, zapcore.AddSync(w), lvl)
	return zap.New(core, zap.AddCaller()), nil
}

type ctxKey struct{}

// WithLogger returns a copy of ctx carrying l. The request-ID middleware uses
// it to hand every handler a logger that already includes the request ID.
func WithLogger(ctx context.Context, l *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the logger stored by WithLogger, or zap's global logger
// (set with zap.ReplaceGlobals in main) if there is none.
func FromContext(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok {
		return l
	}
	return zap.L()
}
