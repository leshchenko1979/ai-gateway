package telemetry

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	provider     *sdktrace.TracerProvider
	providerOnce sync.Once
	initialized  bool
)

// Init configures OpenTelemetry tracing and logging exporters using OTLP env vars.
// It returns a shutdown function that should be called during application cleanup.
func Init(ctx context.Context) (func(context.Context) error, error) {
	var shutdown func(context.Context) error
	var initErr error

	shutdown = func(context.Context) error { return nil }

	providerOnce.Do(func() {
		endpoint := strings.TrimSpace(os.Getenv("OTLP_ENDPOINT"))
		apiKey := strings.TrimSpace(os.Getenv("OTLP_API_KEY"))
		serviceName := strings.TrimSpace(os.Getenv("OTLP_SERVICE_NAME"))
		if serviceName == "" {
			serviceName = "ai-gateway"
		}

		var tp *sdktrace.TracerProvider
		if endpoint == "" || apiKey == "" {
			tp = sdktrace.NewTracerProvider()
			log.Println("Telemetry not configured: set OTLP_ENDPOINT and OTLP_API_KEY to enable tracing/exporting.")
		} else {
			exporter, err := newTraceExporter(ctx, endpoint, apiKey)
			if err != nil {
				initErr = fmt.Errorf("failed to create OTLP exporter: %w", err)
				return
			}
			res, err := buildResource(serviceName)
			if err != nil {
				initErr = fmt.Errorf("failed to build resource: %w", err)
				_ = exporter.Shutdown(ctx)
				return
			}
			tp = sdktrace.NewTracerProvider(
				sdktrace.WithBatcher(exporter),
				sdktrace.WithResource(res),
			)
		}

		provider = tp
		otel.SetTracerProvider(tp)
		initialized = true
		shutdown = func(stopCtx context.Context) error {
			return tp.Shutdown(stopCtx)
		}
		if endpoint != "" && apiKey != "" {
			log.Printf("Telemetry initialized: endpoint=%s service=%s", endpoint, serviceName)
		}
	})

	return shutdown, initErr
}

// Tracer returns a tracer instance for the provided name.
func Tracer(name string) trace.Tracer {
	if !initialized {
		return otel.Tracer(name)
	}
	return otel.Tracer(name)
}

// RecordLog writes log events as OTLP span events to ensure they reach the exporter.
func RecordLog(ctx context.Context, level, message string, fields map[string]interface{}) {
	tracer := Tracer("logger")
	if tracer == nil {
		return
	}

	var attrs []attribute.KeyValue
	if level != "" {
		attrs = append(attrs, attribute.String("log.level", level))
	}
	for key, value := range fields {
		if key == "" || value == nil {
			continue
		}
		attrs = append(attrs, attribute.String(key, fmt.Sprint(value)))
	}

	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		span.AddEvent(message, trace.WithAttributes(attrs...))
		return
	}

	ctx, span = tracer.Start(ctx, "log.record", trace.WithAttributes(attrs...))
	span.AddEvent(message)
	span.End()
}

func newTraceExporter(ctx context.Context, endpoint, apiKey string) (*otlptrace.Exporter, error) {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(apiKey+":"))),
	}

	if extra := strings.TrimSpace(os.Getenv("OTLP_HEADERS")); extra != "" {
		for _, pair := range strings.Split(extra, ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) != 2 {
				continue
			}
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	var addr string
	var opts []otlptracegrpc.Option

	// Special handling for IP address endpoints to set correct TLS server name and ALPN
	if strings.Contains(endpoint, "3.78.14.180") {
		addr = endpoint
		tlsConfig := &tls.Config{
			ServerName: "otlp-gateway-prod-eu-west-2.grafana.net",
			NextProtos: []string{"h2"}, // HTTP/2 for gRPC
		}
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		addr, opts = normalizeEndpoint(endpoint)
	}

	opts = append(opts,
		otlptracegrpc.WithEndpoint(addr),
		otlptracegrpc.WithHeaders(headers),
	)

	return otlptracegrpc.New(ctx, opts...)
}

func normalizeEndpoint(raw string) (string, []otlptracegrpc.Option) {
	opts := make([]otlptracegrpc.Option, 0, 2)
	address := raw
	if strings.HasPrefix(raw, "https://") {
		address = strings.TrimPrefix(raw, "https://")
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
	} else if strings.HasPrefix(raw, "http://") {
		address = strings.TrimPrefix(raw, "http://")
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	} else {
		// Default to TLS
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}
	return strings.TrimSuffix(address, "/"), opts
}

func buildResource(serviceName string) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
	}
	if extra := strings.TrimSpace(os.Getenv("OTLP_RESOURCE_ATTRIBUTES")); extra != "" {
		for _, pair := range strings.Split(extra, ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			if key == "" || value == "" {
				continue
			}
			attrs = append(attrs, attribute.String(key, value))
		}
	}
	return resource.New(context.Background(), resource.WithAttributes(attrs...))
}
