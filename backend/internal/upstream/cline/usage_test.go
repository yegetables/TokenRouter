package cline

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/upstream/usagecontract"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usageview"
	"github.com/stretchr/testify/require"
)

const sampleMe = `{"success": true, "data": {"id": "user-abc", "email": "a@b.c"}}`

const sampleBalance = `{"success": true, "data": {"balance": 123456, "userId": "user-abc"}}`

// Query 先取 /users/me 的 id，再携带 Bearer 请求 /users/{id}/balance，
// 美分余额换算成美元。
func TestQueryFetchesMeThenBalance(t *testing.T) {
	var gotPaths []string
	var gotAuth string
	input := &usagecontract.Request{
		BaseURL: "https://api.cline.bot/api/v1",
		APIKey:  "cline-key",
		Do: func(req *http.Request) (*http.Response, error) {
			gotPaths = append(gotPaths, req.URL.Path)
			gotAuth = req.Header.Get("Authorization")
			body := sampleBalance
			if len(gotPaths) == 1 {
				body = sampleMe
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		},
		Context:      func(ctx context.Context) context.Context { return ctx },
		ApplyHeaders: func(http.Header) {},
	}
	usage, err := (&ClineUsageAdapter{}).Query(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, []string{"/api/v1/users/me", "/api/v1/users/user-abc/balance"}, gotPaths)
	require.Equal(t, "Bearer cline-key", gotAuth)
	require.Equal(t, "cline", usage.Provider)
	require.Equal(t, "balance", usage.Mode)
	require.Equal(t, "USD", usage.Unit)
	require.InDelta(t, 1234.56, *usage.Balance.Remaining, 1e-9)
}

// 信封 success=false、缺 balance 字段或字段类型不对时拒绝整份响应。
func TestParseClineBalanceRejectsInvalid(t *testing.T) {
	for _, body := range []string{
		`{"success": false, "error": "denied"}`,
		`{"success": true, "data": {}}`,
		`{"success": true, "data": {"balance": "12.3"}}`,
		`{"data": null}`,
	} {
		_, err := ParseClineBalance([]byte(body))
		require.ErrorIs(t, err, usageview.ErrUpstreamUsageInvalidResponse)
	}
}

// 官方扩展把 balance 除以 100 展示，0 美分是合法余额。
func TestParseClineBalanceZero(t *testing.T) {
	usage, err := ParseClineBalance([]byte(`{"success": true, "data": {"balance": 0}}`))
	require.NoError(t, err)
	require.InDelta(t, 0.0, *usage.Balance.Remaining, 1e-9)
}

// me 响应里缺 id 时拒绝，防止拼出 /users//balance。
func TestQueryRejectsMeWithoutID(t *testing.T) {
	input := &usagecontract.Request{
		BaseURL: "https://api.cline.bot",
		APIKey:  "k",
		Do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"success": true, "data": {}}`)),
			}, nil
		},
		Context:      func(ctx context.Context) context.Context { return ctx },
		ApplyHeaders: func(http.Header) {},
	}
	_, err := (&ClineUsageAdapter{}).Query(context.Background(), input)
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageInvalidResponse)
}

// 401 映射为认证失败错误。
func TestQueryAuthFailureMapsToAuthError(t *testing.T) {
	input := &usagecontract.Request{
		BaseURL: "https://api.cline.bot",
		APIKey:  "expired",
		Do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"success": false, "error": "Unauthorized"}`)),
			}, nil
		},
		Context:      func(ctx context.Context) context.Context { return ctx },
		ApplyHeaders: func(http.Header) {},
	}
	_, err := (&ClineUsageAdapter{}).Query(context.Background(), input)
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageAuthFailed)
}

// 站点根、/api 结尾、/api/v1 结尾三种 base_url 形态拼出同一个余额端点。
func TestUsageEndpointNormalizesBaseURL(t *testing.T) {
	const expected = "https://api.cline.bot/api/v1/users/me"
	for _, base := range []string{
		"https://api.cline.bot",
		"https://api.cline.bot/",
		"https://api.cline.bot/api",
		"https://api.cline.bot/api/",
		"https://api.cline.bot/api/v1",
		"https://api.cline.bot/api/v1/",
	} {
		endpoint, err := usageEndpoint(base, "/users/me")
		require.NoError(t, err)
		require.Equal(t, expected, endpoint)
	}
}
