// Package logging installs a slog handler that removes secrets before any
// record is written. Code should not log secrets in the first place; this is
// the second line of defense for attributes and messages that slip through.
package logging

import (
	"context"
	"io"
	"log/slog"
	"regexp"
	"strings"
)

var (
	sensitiveKeys = []string{"password", "passphrase", "privatekey", "private_key", "secret", "token", "bootstrap", "cookie", "csrf", "credential", "authorization", "answer", "response"}
	sensitiveText = []*regexp.Regexp{
		regexp.MustCompile(`#bootstrap=[A-Za-z0-9_\-]+`),
		regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(-----END [A-Z ]*PRIVATE KEY-----|$)`),
		regexp.MustCompile(`(?i)(sshwb_session|__Host-sshwb_session)=[^;\s]+`),
		regexp.MustCompile(`(?i)(X-CSRF-Token:\s*)\S+`),
	}
)

const redacted = "[REDACTED]"

func sensitive(key string) bool {
	lower := strings.ToLower(key)
	for _, candidate := range sensitiveKeys {
		if strings.Contains(lower, candidate) {
			return true
		}
	}
	return false
}

func Scrub(value string) string {
	for _, pattern := range sensitiveText {
		value = pattern.ReplaceAllStringFunc(value, func(match string) string {
			if strings.HasPrefix(match, "#bootstrap=") {
				return "#bootstrap=" + redacted
			}
			return redacted
		})
	}
	return value
}

func clean(attribute slog.Attr) slog.Attr {
	if sensitive(attribute.Key) {
		return slog.String(attribute.Key, redacted)
	}
	attribute.Value = attribute.Value.Resolve()
	switch attribute.Value.Kind() {
	case slog.KindString:
		return slog.String(attribute.Key, Scrub(attribute.Value.String()))
	case slog.KindGroup:
		group := attribute.Value.Group()
		cleaned := make([]any, 0, len(group))
		for _, member := range group {
			cleaned = append(cleaned, clean(member))
		}
		return slog.Group(attribute.Key, cleaned...)
	case slog.KindAny:
		// Structs, maps and byte slices may embed credentials under field
		// names this handler cannot see; only errors are printed (scrubbed).
		if err, ok := attribute.Value.Any().(error); ok {
			return slog.String(attribute.Key, Scrub(err.Error()))
		}
		return slog.String(attribute.Key, "[omitted]")
	}
	return attribute
}

type handler struct {
	next slog.Handler
}

func (redactor handler) Enabled(ctx context.Context, level slog.Level) bool {
	return redactor.next.Enabled(ctx, level)
}

func (redactor handler) Handle(ctx context.Context, record slog.Record) error {
	cleaned := slog.NewRecord(record.Time, record.Level, Scrub(record.Message), record.PC)
	record.Attrs(func(attribute slog.Attr) bool {
		cleaned.AddAttrs(clean(attribute))
		return true
	})
	return redactor.next.Handle(ctx, cleaned)
}

func (redactor handler) WithAttrs(attributes []slog.Attr) slog.Handler {
	cleaned := make([]slog.Attr, 0, len(attributes))
	for _, attribute := range attributes {
		cleaned = append(cleaned, clean(attribute))
	}
	return handler{next: redactor.next.WithAttrs(cleaned)}
}

func (redactor handler) WithGroup(name string) slog.Handler {
	return handler{next: redactor.next.WithGroup(name)}
}

func New(next slog.Handler) slog.Handler {
	return handler{next: next}
}

// Install makes the redacting handler the process default.
func Install(output io.Writer) {
	slog.SetDefault(slog.New(New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: slog.LevelInfo}))))
}
