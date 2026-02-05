package telemetry

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
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

		// Try standard OTEL env vars first, then OTLP prefixes for compatibility
		serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
		if serviceName == "" {
			serviceName = strings.TrimSpace(os.Getenv("OTLP_SERVICE_NAME"))
		}
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
	// For Grafana Cloud, the API key might be a "glc_" prefixed token.
	// Grafana Cloud OTLP HTTP requires Basic auth with InstanceID as username and API Key as password.
	// We'll try to extract the InstanceID from the glc_ token (which contains base64 encoded JSON).
	authHeader := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(apiKey+":")))
	if strings.HasPrefix(apiKey, "glc_") {
		tokenPart := strings.TrimPrefix(apiKey, "glc_")
		if decoded, err := base64.StdEncoding.DecodeString(tokenPart); err == nil {
			// Try to extract the instance ID from the "o" (org) or "n" (name) fields.
			// In Grafana Cloud, the OTLP instance ID is often the Org ID or Stack ID.
			instanceID := ""
			if idx := strings.Index(string(decoded), "\"n\":\"stack-"); idx != -1 {
				idPart := string(decoded)[idx+11:]
				if endIdx := strings.Index(idPart, "-"); endIdx != -1 {
					instanceID = idPart[:endIdx]
				}
			} else if idx := strings.Index(string(decoded), "\"o\":\""); idx != -1 {
				idPart := string(decoded)[idx+5:]
				if endIdx := strings.Index(idPart, "\""); endIdx != -1 {
					instanceID = idPart[:endIdx]
				}
			}
			if instanceID != "" {
				authHeader = fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(instanceID+":"+apiKey)))
			}
		}
	}

	headers := map[string]string{
		"Authorization": authHeader,
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

	addr, opts := normalizeEndpoint(endpoint)
	opts = append(opts,
		otlptracehttp.WithEndpoint(addr),
		otlptracehttp.WithHeaders(headers),
	)

	return otlptracehttp.New(ctx, opts...)
}

func normalizeEndpoint(raw string) (string, []otlptracehttp.Option) {
	opts := make([]otlptracehttp.Option, 0, 2)
	address := raw

	// Handle protocol
	if strings.HasPrefix(raw, "https://") {
		address = strings.TrimPrefix(raw, "https://")
		// TLS is default for https://
	} else if strings.HasPrefix(raw, "http://") {
		address = strings.TrimPrefix(raw, "http://")
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	// In OTLP HTTP, the exporter appends /v1/traces by default.
	// If the user provided a path, we must ensure it includes /v1/traces.
	if parts := strings.SplitN(address, "/", 2); len(parts) > 1 {
		address = parts[0]
		path := "/" + strings.TrimSuffix(parts[1], "/")
		if !strings.HasSuffix(path, "/v1/traces") {
			path = path + "/v1/traces"
		}
		opts = append(opts, otlptracehttp.WithURLPath(path))
	}

	return address, opts
}

func buildResource(serviceName string) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
	}

	// Try standard OTEL env var first, then OTLP prefix for compatibility
	extra := strings.TrimSpace(os.Getenv("OTEL_RESOURCE_ATTRIBUTES"))
	if extra == "" {
		extra = strings.TrimSpace(os.Getenv("OTLP_RESOURCE_ATTRIBUTES"))
	}

	if extra != "" {
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
