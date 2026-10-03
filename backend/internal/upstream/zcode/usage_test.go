package zcode

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/upstream/usagecontract"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usageview"
	"github.com/stretchr/testify/require"
)

const sampleQuota = `{
 "provider": "bigmodel",
 "serverTime": 1791019177,
 "balances": [
  {"showName": "GLM-5.3-Flash", "remainingUnits": 93739382, "totalUnits": 100000000, "usedUnits": 6260618, "unitType": "token", "expiresAt": 1791043200},
  {"showName": "GLM-5.3", "remainingUnits": 2994743, "totalUnits": 3000000, "usedUnits": 5257, "unitType": "token", "expiresAt": 1791043199},
  {"showName": "GLM-5.3-Flash", "remainingUnits": 0, "totalUnits": 5000000, "usedUnits": 5000000, "unitType": "token", "expiresAt": 1791043199}
 ],
 "errors": ["coding: 500 当前用户不存在coding plan"]
}`

func TestParseZCodeUsage(t *testing.T) {
	usage, err := ParseZCodeUsage([]byte(sampleQuota))
	require.NoError(t, err)
	require.Equal(t, "zcode", usage.Provider)
	require.Equal(t, "limits", usage.Mode)
	require.Equal(t, "TOKENS", usage.Unit)
	require.Len(t, usage.Limits, 3)

	first := usage.Limits[0]
	require.Equal(t, "GLM-5.3-Flash", first.Name)
	require.Equal(t, 6260618.0, *first.Used)
	require.Equal(t, 100000000.0, *first.Limit)
	require.Equal(t, 93739382.0, *first.Remaining)
	require.Equal(t, time.Unix(1791043200, 0).UTC(), *first.ResetAt)

	require.Equal(t, "GLM-5.3", usage.Limits[1].Name)
	// 同名第二桶追加序号，避免面板无法区分套餐桶与每日桶。
	require.Equal(t, "GLM-5.3-Flash #2", usage.Limits[2].Name)
	require.Equal(t, 0.0, *usage.Limits[2].Remaining)
	// errors 字段是常态噪音（无 coding plan），不影响解析。
}

func TestParseZCodeUsageRejectsInconsistentQuota(t *testing.T) {
	_, err := ParseZCodeUsage([]byte(
		`{"balances":[{"showName":"X","remainingUnits":5,"totalUnits":100,"usedUnits":3,"unitType":"token"}]}`))
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageInvalidResponse)
}

func TestParseZCodeUsageRejectsEmpty(t *testing.T) {
	_, err := ParseZCodeUsage([]byte(`{"balances":[]}`))
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageInvalidResponse)
}

// Query 必须剥掉 Base URL 的版本段、携带 Bearer 认证请求站点根的 /quota。
func TestQueryUsesRootQuotaEndpointWithBearer(t *testing.T) {
	var gotPath, gotAuth string
	input := &usagecontract.Request{
		BaseURL: "http://proxy:12346/v1",
		APIKey:  "zcode",
		Do: func(req *http.Request) (*http.Response, error) {
			gotPath = req.URL.Path
			gotAuth = req.Header.Get("Authorization")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(sampleQuota)),
			}, nil
		},
		Context:      func(ctx context.Context) context.Context { return ctx },
		ApplyHeaders: func(http.Header) {},
	}
	adapter := &ZCodeUsageAdapter{}
	usage, err := adapter.Query(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "/quota", gotPath)
	require.Equal(t, "Bearer zcode", gotAuth)
	require.Len(t, usage.Limits, 3)
}

func TestQueryAuthFailureMapsToAuthError(t *testing.T) {
	input := &usagecontract.Request{
		BaseURL: "http://proxy:12346",
		APIKey:  "wrong",
		Do: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Invalid or missing proxy API key"}}`)),
			}, nil
		},
		Context:      func(ctx context.Context) context.Context { return ctx },
		ApplyHeaders: func(http.Header) {},
	}
	_, err := (&ZCodeUsageAdapter{}).Query(context.Background(), input)
	require.ErrorIs(t, err, usageview.ErrUpstreamUsageAuthFailed)
}
