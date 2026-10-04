package selfcapture

import (
	"strings"
	"testing"
)

func TestCaptureHeadersRedactsSensitiveKeys(t *testing.T) {
	headers := CaptureRequestHeaders(map[string][]string{
		"Authorization": {"Bearer sk-secret"},
		"Cookie":        {"session=abc"},
		"User-Agent":    {"claude-cli/1.0"},
	})
	for _, secret := range []string{"sk-secret", "session=abc"} {
		if strings.Contains(headers, secret) {
			t.Fatalf("敏感值泄漏到请求头快照: %s in %s", secret, headers)
		}
	}
	if !strings.Contains(headers, "claude-cli/1.0") {
		t.Fatalf("普通请求头不应被脱敏: %s", headers)
	}
}

func TestCaptureBodyTruncates(t *testing.T) {
	body := strings.Repeat("a", 2<<20)
	text, truncated := CaptureResponseBody([]byte(body))
	if !truncated || len(text) != bodyLimitBytes {
		t.Fatalf("期望截断到 %d 字节, got %d (truncated=%v)", bodyLimitBytes, len(text), truncated)
	}
	small, truncated := CaptureRequestBody([]byte(`{"model":"gpt"}`))
	if truncated || small != `{"model":"gpt"}` {
		t.Fatalf("小请求体不应被改写: %q truncated=%v", small, truncated)
	}
}

func TestStoreNilSafety(t *testing.T) {
	var store *Store
	store.Enqueue(Entry{ClientRequestID: "x"})
	if store.Disabled() {
		t.Fatal("nil store 不应报告停用")
	}
}
