package telemetry

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

type Config struct {
	Enabled      bool
	ServiceName  string
	OTLPEndpoint string
	SampleRatio  float64
}

func (c Config) Normalized(fallbackServiceName string) Config {
	c.ServiceName = strings.TrimSpace(c.ServiceName)
	if c.ServiceName == "" {
		c.ServiceName = strings.TrimSpace(fallbackServiceName)
	}
	if c.ServiceName == "" {
		c.ServiceName = "watch-together"
	}
	if c.SampleRatio < 0 {
		c.SampleRatio = 0
	}
	if c.SampleRatio > 1 {
		c.SampleRatio = 1
	}
	return c
}

type ShutdownFunc func(context.Context) error

func Start(ctx context.Context, config Config) (ShutdownFunc, error) {
	config = config.Normalized("")
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if !config.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	options := []otlptracehttp.Option{}
	if strings.TrimSpace(config.OTLPEndpoint) != "" {
		options = append(options, otlptracehttp.WithEndpoint(strings.TrimSpace(config.OTLPEndpoint)))
	}
	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SampleRatio)),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
		)),
	)
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

func Shutdown(ctx context.Context, shutdown ShutdownFunc) error {
	if shutdown == nil {
		return nil
	}
	if err := shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
