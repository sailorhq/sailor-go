package sailor

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

type slogLogger struct{ l *slog.Logger }

func newDefaultLogger() opts.Logger {
	return slogLogger{l: slog.New(slog.NewTextHandler(os.Stderr, nil))}
}

func (s slogLogger) Log(level opts.Level, event string, kv ...any) {
	var lvl slog.Level
	switch level {
	case opts.LevelDebug:
		lvl = slog.LevelDebug
	case opts.LevelWarn:
		lvl = slog.LevelWarn
	case opts.LevelError:
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	s.l.Log(context.Background(), lvl, event, append([]any{"event", event}, kv...)...)
}

// shortDigest truncates "sha256:<hex>" to the prefix plus 12 hex characters
// (SAILOR_CONSUMER.md CONS-LOG-11).
func shortDigest(d string) string {
	const prefix = "sha256:"
	if !strings.HasPrefix(d, prefix) {
		return d
	}
	hexPart := d[len(prefix):]
	if len(hexPart) > 12 {
		hexPart = hexPart[:12]
	}
	return prefix + hexPart
}

// redactURL removes userinfo from a URL before it is logged (CONS-LOG-10).
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable>"
	}
	u.User = nil
	return u.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
