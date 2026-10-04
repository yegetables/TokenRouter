package selfcapture

import (
	"encoding/json"
	"strings"
	"testing"
)

// 前端按 snake_case 读取载荷字段；结构体漏掉 JSON tag 会让弹窗永远显示“未捕获”。
func TestEntryJSONFieldNames(t *testing.T) {
	raw, err := json.Marshal(Entry{ClientRequestID: "x", RequestBody: "{}", ResponseBody: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"client_request_id", "request_headers", "response_headers",
		"request_body", "response_body", "request_body_truncated",
		"response_body_truncated", "status_code", "request_path",
	} {
		if !strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("返回 JSON 缺少字段 %q: %s", key, raw)
		}
	}
}

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
