package provider_test

import (
	"testing"

	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/stretchr/testify/require"
)

func TestEffectiveUpstreamUsageConfigDefaultsAndNormalization(t *testing.T) {
	provider := &providercore.Record{Type: capability.ProviderTypeAPIKey}
	config, err := providercore.EffectiveUpstreamUsageConfig(provider)
	require.NoError(t, err)
	require.Equal(t, providercore.UpstreamUsageQueryConfig{Enabled: true, Adapter: providercore.UpstreamUsageAdapterSub2API}, config)

	extra := map[string]any{providercore.UpstreamUsageQueryExtraKey: map[string]any{
		"enabled":  false,
		"adapter":  providercore.UpstreamUsageAdapterNewAPI,
		"base_url": "https://usage.example/v1",
	}}
	require.NoError(t, providercore.NormalizeUpstreamUsageExtra(extra))
	require.Equal(t, map[string]any{
		"enabled":  false,
		"adapter":  providercore.UpstreamUsageAdapterNewAPI,
		"base_url": "https://usage.example/v1",
	}, extra[providercore.UpstreamUsageQueryExtraKey])

	bad := map[string]any{providercore.UpstreamUsageQueryExtraKey: map[string]any{"api_key": "secret"}}
	require.Error(t, providercore.NormalizeUpstreamUsageExtra(bad))

	disabledProvider := &providercore.Record{Extra: map[string]any{providercore.UpstreamUsageQueryExtraKey: map[string]any{"enabled": false}}}
	disabled, err := providercore.EffectiveUpstreamUsageConfig(disabledProvider)
	require.NoError(t, err)
	require.False(t, disabled.Enabled)
	require.Equal(t, providercore.UpstreamUsageAdapterSub2API, disabled.Adapter)

	unknown := map[string]any{providercore.UpstreamUsageQueryExtraKey: map[string]any{"adapter": "custom-script"}}
	require.ErrorIs(t, providercore.NormalizeUpstreamUsageExtra(unknown), providercore.ErrUpstreamUsageConfigInvalid)
	_, err = providercore.EffectiveUpstreamUsageConfig(&providercore.Record{Extra: unknown})
	require.ErrorIs(t, err, providercore.ErrUpstreamUsageUnsupported)

	unsafeURL := map[string]any{providercore.UpstreamUsageQueryExtraKey: map[string]any{
		"base_url": "https://user:secret@gateway.example/v1?token=secret",
	}}
	require.ErrorIs(t, providercore.NormalizeUpstreamUsageExtra(unsafeURL), providercore.ErrUpstreamUsageConfigInvalid)
	_, ok := providercore.NormalizedUpstreamUsageConfigValue(map[string]any{"api_key": "secret"})
	require.False(t, ok)

	require.Equal(t, []providercore.UpstreamUsageAdapterOption{
		{Name: providercore.UpstreamUsageAdapterSub2API, Label: "Sub2API / TokenRouter"},
		{Name: providercore.UpstreamUsageAdapterNewAPI, Label: "New API"},
		{Name: providercore.UpstreamUsageAdapterZivv, Label: "Zivv"},
		{Name: providercore.UpstreamUsageAdapterZCode, Label: "ZCode Start Plan"},
	}, providercore.UpstreamUsageAdapterOptions())
}
