package provider

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

func TestEffectiveUpstreamUsageConfigDefaultsAndNormalization(t *testing.T) {
	provider := &Record{Type: capability.ProviderTypeAPIKey}
	config, err := EffectiveUpstreamUsageConfig(provider)
	require.NoError(t, err)
	require.Equal(t, UpstreamUsageQueryConfig{Enabled: true, Adapter: UpstreamUsageAdapterSub2API}, config)

	extra := map[string]any{UpstreamUsageQueryExtraKey: map[string]any{
		"enabled":  false,
		"adapter":  UpstreamUsageAdapterNewAPI,
		"base_url": "https://usage.example/v1",
	}}
	require.NoError(t, NormalizeUpstreamUsageExtra(extra))
	require.Equal(t, map[string]any{
		"enabled":  false,
		"adapter":  UpstreamUsageAdapterNewAPI,
		"base_url": "https://usage.example/v1",
	}, extra[UpstreamUsageQueryExtraKey])

	bad := map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"api_key": "secret"}}
	require.Error(t, NormalizeUpstreamUsageExtra(bad))

	disabledProvider := &Record{Extra: map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"enabled": false}}}
	disabled, err := EffectiveUpstreamUsageConfig(disabledProvider)
	require.NoError(t, err)
	require.False(t, disabled.Enabled)
	require.Equal(t, UpstreamUsageAdapterSub2API, disabled.Adapter)

	unknown := map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"adapter": "custom-script"}}
	require.ErrorIs(t, NormalizeUpstreamUsageExtra(unknown), ErrUpstreamUsageConfigInvalid)
	_, err = EffectiveUpstreamUsageConfig(&Record{Extra: unknown})
	require.ErrorIs(t, err, ErrUpstreamUsageUnsupported)

	unsafeURL := map[string]any{UpstreamUsageQueryExtraKey: map[string]any{
		"base_url": "https://user:secret@gateway.example/v1?token=secret",
	}}
	require.ErrorIs(t, NormalizeUpstreamUsageExtra(unsafeURL), ErrUpstreamUsageConfigInvalid)
	_, ok := NormalizedUpstreamUsageConfigValue(map[string]any{"api_key": "secret"})
	require.False(t, ok)

	require.Equal(t, []UpstreamUsageAdapterOption{
		{Name: UpstreamUsageAdapterSub2API, Label: "Sub2API / TokenRouter"},
		{Name: UpstreamUsageAdapterNewAPI, Label: "New API"},
		{Name: UpstreamUsageAdapterZivv, Label: "Zivv"},
		{Name: UpstreamUsageAdapterZCode, Label: "ZCode Start Plan"},
		{Name: UpstreamUsageAdapterCline, Label: "Cline"},
		{Name: UpstreamUsageAdapterClinePass, Label: "ClinePass"},
	}, UpstreamUsageAdapterOptions())
}
