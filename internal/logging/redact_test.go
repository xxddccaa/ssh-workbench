package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestSecretsNeverReachLogOutput(test *testing.T) {
	var output bytes.Buffer
	logger := slog.New(New(slog.NewTextHandler(&output, nil)))
	logger.Info("open http://127.0.0.1:8622/#bootstrap=abcDEF123_-",
		"password", "hunter2-secret",
		"privateKey", "material",
		slog.Group("request", "X-CSRF-Token", "csrf-value", "path", "/workspace"),
		"error", errors.New("parse -----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY----- failed"),
		"cookie", "sshwb_session=session-value",
	)
	logger.With("passphrase", "with-attribute").Warn("sshwb_session=other-session; Path=/")
	type target struct{ Password string }
	logger.Info("structured", "target", target{Password: "struct-secret"}, "blob", []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nBBBB"), "count", 3, "lazy", lazy{})
	text := output.String()
	for _, secret := range []string{"abcDEF123", "hunter2-secret", "material", "csrf-value", "AAAA", "session-value", "with-attribute", "other-session", "struct-secret", "BBBB", "lazy-secret"} {
		if strings.Contains(text, secret) {
			test.Fatalf("secret %q leaked: %s", secret, text)
		}
	}
	if !strings.Contains(text, "/workspace") || !strings.Contains(text, "#bootstrap=[REDACTED]") || !strings.Contains(text, "count=3") {
		test.Fatalf("non-secret context lost: %s", text)
	}
}

type lazy struct{}

func (lazy) LogValue() slog.Value {
	return slog.AnyValue(struct{ Token string }{Token: "lazy-secret"})
}
