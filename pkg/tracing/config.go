package tracing

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Config controls tracing. All fields are optional; tracing is off by default
// so the binary runs identically when no TRACING_* env vars are set.
type Config struct {
	Enabled        bool    `envconfig:"TRACING_ENABLED"         default:"false"`
	OTLPEndpoint   string  `envconfig:"TRACING_OTLP_ENDPOINT"   default:"localhost:4317"`
	SampleRatio    float64 `envconfig:"TRACING_SAMPLE_RATIO"    default:"1.0"`
	CaptureContent bool    `envconfig:"TRACING_CAPTURE_CONTENT" default:"true"`
	ServiceName    string  `envconfig:"TRACING_SERVICE_NAME"    default:"toxicbot"`
}

func ParseConfig() (Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return c, fmt.Errorf("envconfig.Process error: %w", err)
	}
	return c, nil
}
