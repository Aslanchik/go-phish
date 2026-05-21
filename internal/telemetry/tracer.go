package telemetry

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Init configures the global OTel tracer provider.
// It enables the OTLP HTTP exporter unless OTEL_EXPORTER_OTLP_ENDPOINT is set to
// an empty string, and the file exporter when OTEL_FILE_EXPORTER_PATH is non-empty.
// On any setup error it installs a no-op provider and returns the error; the caller
// should log and continue — the investigation is not aborted.
func Init(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var processors []sdktrace.SpanProcessor
	var fileHandle *os.File

	// Step 1–2: OTLP HTTP exporter.
	endpoint, otlpSet := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if !otlpSet || endpoint != "" {
		exp, expErr := otlptracehttp.New(ctx)
		if expErr != nil {
			setNoop()
			return noopShutdown, fmt.Errorf("otlp exporter: %w", expErr)
		}
		processors = append(processors, sdktrace.NewBatchSpanProcessor(exp))
	}

	// Step 3: file fallback exporter.
	if path := os.Getenv("OTEL_FILE_EXPORTER_PATH"); path != "" {
		f, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if openErr != nil {
			log.Printf("warn: telemetry file exporter: cannot open %s: %v — skipping", path, openErr)
		} else {
			fileHandle = f
			fileExp, fileExpErr := stdouttrace.New(stdouttrace.WithWriter(f))
			if fileExpErr != nil {
				log.Printf("warn: telemetry file exporter: %v — skipping", fileExpErr)
				_ = f.Close()
				fileHandle = nil
			} else {
				processors = append(processors, sdktrace.NewSimpleSpanProcessor(fileExp))
			}
		}
	}

	// Step 4: build and register the provider.
	opts := []sdktrace.TracerProviderOption{sdktrace.WithSampler(sdktrace.AlwaysSample())}
	for _, p := range processors {
		opts = append(opts, sdktrace.WithSpanProcessor(p))
	}
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)

	return func(shutCtx context.Context) error {
		shutErr := tp.Shutdown(shutCtx)
		if fileHandle != nil {
			_ = fileHandle.Close()
		}
		return shutErr
	}, nil
}

// Version returns the first 12 hex chars of the git commit SHA embedded at build
// time, or "dev" when build info is unavailable or vcs.revision is absent.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			if len(s.Value) >= 12 {
				return s.Value[:12]
			}
			if s.Value != "" {
				return s.Value
			}
			return "dev"
		}
	}
	return "dev"
}

func setNoop() {
	otel.SetTracerProvider(trace.NewNoopTracerProvider())
}

func noopShutdown(_ context.Context) error { return nil }
