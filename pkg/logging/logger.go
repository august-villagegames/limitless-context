package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/config"
)

// Options describe how to configure a logger instance.
type Options struct {
	Level  string
	Format string
	Output io.Writer
}

// New creates a structured logger backed by Go's slog package.
func New(opts Options) (*slog.Logger, error) {
	lvl, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	out := opts.Output
	if out == nil {
		out = os.Stderr
	}

	handlerOpts := slog.HandlerOptions{
		Level:       lvl,
		ReplaceAttr: replaceTimeAttr,
	}

	format := strings.ToLower(strings.TrimSpace(opts.Format))
	var handler slog.Handler
	switch format {
	case "", "json":
		handler = slog.NewJSONHandler(out, &handlerOpts)
	case "console", "text":
		handler = slog.NewTextHandler(out, &handlerOpts)
	default:
		return nil, fmt.Errorf("unsupported log format %q", opts.Format)
	}

	return slog.New(handler), nil
}

func parseLevel(level string) (slog.Leveler, error) {
	trimmed, err := configNormalize(level)
	if err != nil {
		return nil, err
	}

	var lvl slog.Level
	switch trimmed {
	case "info":
		lvl = slog.LevelInfo
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("unhandled log level %q", trimmed)
	}

	var levelVar slog.LevelVar
	levelVar.Set(lvl)
	return &levelVar, nil
}

func configNormalize(level string) (string, error) {
	normalized, err := config.NormalizeLogLevel(level)
	if err != nil {
		return "", err
	}
	return normalized, nil
}

func replaceTimeAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.TimeKey && attr.Value.Kind() == slog.KindTime {
		attr.Value = slog.StringValue(attr.Value.Time().UTC().Format(time.RFC3339))
	}
	return attr
}
