package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/reijo1337/ToxicBot"

// captureContent gates whether prompt/answer text is attached to spans. It is
// written once by Setup at startup and only read afterwards.
var captureContent = true

// Provider owns the SDK tracer provider and exposes a single Shutdown handle.
type Provider struct {
	tp *sdktrace.TracerProvider
}

// Setup builds a tracer provider from cfg and installs it globally. When
// cfg.Enabled is false it installs nothing (the global no-op tracer remains)
// and returns a Provider whose Shutdown is a no-op.
func Setup(ctx context.Context, cfg Config) (*Provider, error) {
	captureContent = cfg.CaptureContent
	if !cfg.Enabled {
		return &Provider{}, nil
	}

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("otlptracegrpc.New: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", cfg.ServiceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("resource.New: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(tp)
	return &Provider{tp: tp}, nil
}

// Shutdown flushes and stops the provider. Safe on a disabled Provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.tp == nil {
		return nil
	}
	return p.tp.Shutdown(ctx)
}

// Tracer returns the bot's tracer from the global provider.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// ContentAttr returns a string attribute carrying value only when content
// capture is enabled; otherwise it records the rune length so spans stay
// useful without storing the message body.
func ContentAttr(key, value string) attribute.KeyValue {
	if captureContent {
		return attribute.String(key, value)
	}
	return attribute.Int(key+".len", len([]rune(value)))
}
