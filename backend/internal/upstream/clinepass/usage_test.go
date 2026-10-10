package clinepass

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/upstream/usagecontract"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usageview"
)

const sampleLimits = `{
 "data": {
  "limits": [
   {"type": "five_hour", "percentUsed": 35.5, "resetsAt": 1791043200000},
   {"type": "weekly", "percentUsed": "12.3", "resetsAt": 1791043199},
   {"type": "monthly", "percentUsed": 0, "resetsAt": "2026-10-31T15:59:59Z"}
  ]
 }
}`

func TestParseClinePassUsage(t *testing.T) {
	usage, err := ParseClinePassUsage([]byte(sampleLimits))
	require.NoError(t, err)
	require.Equal(t, "cline_pass", usage.Provider)
	require.Equal(t, "limits", usage.Mode)
	require.Equal(t, "PERCENT", usage.Unit)
	require.Len(t, usage.Limits, 3)

	fiveHour := usage.Limits[0]
	require.Equal(t, "5h", fiveHour.Name)
	require.Equal(t, 35.5, *fiveHour.Used)
	require.Equal(t, 100.0, *fiveHour.Limit)
	require.InDelta(t, 64.5, *fiveHour.Remaining, 1e-9)
	// 毫秒时间戳自动归一为秒。
	require.Equal(t, time.Unix(1791043200, 0).UTC(), *fiveHour.ResetAt)

	weekly := usage.Limits[1]
	require.Equal(t, "weekly", weekly.Name)
	require.Equal(t, 12.3, *weekly.Used)
	require.InDelta(t, 87.7, *weekly.Remaining, 1e-9)
	require.Equal(t, time.Unix(1791043199, 0).UTC(), *weekly.ResetAt)

	monthly := usage.Limits[2]
	require.Equal(t, "monthly", monthly.Name)
	require.Equal(t, 0.0, *monthly.Used)
	require.Equal(t, 100.0, *monthly.Remaining)
	require.Equal(t, "2026-10-31T15:59:59Z", monthly.ResetAt.Format(time.RFC3339))
}

func TestParseClinePassSkipsUnknownWindows(t *testing.T) {
	body := `{"data":{"limits":[{"type":"five_hour","percentUsed":10},{"type":"daily","percentUsed":99}]}}`
	usage, err := ParseClinePassUsage([]byte(body))
	require.NoError(t, err)
	require.Len(t, usage.Limits, 1)
	require.Equal(t, "5h", usage.Limits[0].Name)
}

func TestParseClinePassRejectsOutOfRangePercent(t *testing.T) {
	_, err := ParseClinePassUsage([]byte(`{"data":{"limits":[{"type":"weekly","percentUsed":120}]}}`))
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageInvalidResponse)
}

func TestParseClinePassRejectsDuplicateWindows(t *testing.T) {
	body := `{"data":{"limits":[{"type":"weekly","percentUsed":1},{"type":"weekly","percentUsed":2}]}}`
	_, err := ParseClinePassUsage([]byte(body))
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageInvalidResponse)
}

func TestParseClinePassRejectsEmpty(t *testing.T) {
	_, err := ParseClinePassUsage([]byte(`{"data":{"limits":[]}}`))
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageInvalidResponse)
	_, err = ParseClinePassUsage([]byte(`{"data":{}}`))
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageInvalidResponse)
}

// Query 必须剥掉 Base URL 的版本段、携带 Bearer 认证请求 plan usage-limits 端点。
func TestQueryUsesPlanUsageLimitsEndpointWithBearer(t *testing.T) {
	var gotPath, gotAuth string
	input := &usagecontract.Request{
		BaseURL: "https://api.cline.bot/v1",
		APIKey:  "cp-key",
		Do: func(req *http.Request) (*http.Response, error) {
			gotPath = req.URL.Path
			gotAuth = req.Header.Get("Authorization")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(sampleLimits)),
			}, nil
		},
		Context:      func(ctx context.Context) context.Context { return ctx },
		ApplyHeaders: func(http.Header) {},
	}
	usage, err := (&ClinePassUsageAdapter{}).Query(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "/api/v1/users/me/plan/usage-limits", gotPath)
	require.Equal(t, "Bearer cp-key", gotAuth)
	require.Len(t, usage.Limits, 3)
}

func TestQueryAuthFailureMapsToAuthError(t *testing.T) {
	input := &usagecontract.Request{
		BaseURL: "https://api.cline.bot",
		APIKey:  "expired",
		Do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"error":"Unauthorized"}`)),
			}, nil
		},
		Context:      func(ctx context.Context) context.Context { return ctx },
		ApplyHeaders: func(http.Header) {},
	}
	_, err := (&ClinePassUsageAdapter{}).Query(context.Background(), input)
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageAuthFailed)
}

// 站点根、/api 结尾、/api/v1 结尾三种 base_url 形态拼出同一个用量端点。
func TestUsageEndpointNormalizesBaseURL(t *testing.T) {
	const expected = "https://api.cline.bot/api/v1/users/me/plan/usage-limits"
	for _, base := range []string{
		"https://api.cline.bot",
		"https://api.cline.bot/",
		"https://api.cline.bot/api",
		"https://api.cline.bot/api/",
		"https://api.cline.bot/api/v1",
		"https://api.cline.bot/api/v1/",
	} {
		endpoint, err := usageEndpoint(base)
		require.NoError(t, err)
		require.Equal(t, expected, endpoint)
	}
}
