package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := ParseConfig()
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "localhost:4317", cfg.OTLPEndpoint)
	assert.InDelta(t, 1.0, cfg.SampleRatio, 0.0001)
	assert.True(t, cfg.CaptureContent)
	assert.Equal(t, "toxicbot", cfg.ServiceName)
}

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestSetup_Disabled_NoopShutdown(t *testing.T) {
	p, err := Setup(context.Background(), Config{Enabled: false})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NoError(t, p.Shutdown(context.Background()))
}

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestContentAttr_RespectsCaptureFlag(t *testing.T) {
	captureContent = true
	a := ContentAttr("gen_ai.input", "привет")
	assert.Equal(t, "gen_ai.input", string(a.Key))
	assert.Equal(t, "привет", a.Value.AsString())

	captureContent = false
	defer func() { captureContent = true }()
	b := ContentAttr("gen_ai.input", "привет")
	assert.Equal(t, "gen_ai.input.len", string(b.Key))
	assert.Equal(t, int64(6), b.Value.AsInt64())
}
