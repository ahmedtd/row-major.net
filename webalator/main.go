package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/profiler"
	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel/api/global"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	listen = flag.String("listen", "0.0.0.0:80", "Where should we listen for incoming connections?")
)

func main() {
	// Cloud Profiler initialization, best done as early as possible.
	if err := profiler.Start(profiler.Config{
		Service:        "webalator",
		ServiceVersion: "0.0.1",
	}); err != nil {
		log.Fatalf("Error initializing profiler: %v", err)
	}

	// Create Cloud Trace exporter.
	exporter, err := texporter.NewExporter()
	if err != nil {
		log.Fatalf("Error initializing Cloud Trace exporter: %v", err)
	}

	// Create trace provider with the exporter.
	config := sdktrace.Config{DefaultSampler: sdktrace.ProbabilitySampler(0.5)}
	tp, err := sdktrace.NewProvider(sdktrace.WithConfig(config), sdktrace.WithSyncer(exporter))
	if err != nil {
		log.Fatalf("Error registering Cloud Trace exporter: %v", err)
	}
	global.SetTraceProvider(tp)

	serveMux := http.NewServeMux()
	serveMux.Register("/", basicHandler)

	server := &http.Server{
		Addr: *listen,

		Handler: serveMux,

		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error while serving http: %v", err)
	}
}

func basicHandler(w http.ResponseWriter, req *http.Request) {
	w.Write("Hello, world!")
}
