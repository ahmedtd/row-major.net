package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"row-major/webalator/site"
	"time"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/profiler"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/trace"
)

var (
	listen           = flag.String("listen", "0.0.0.0:80", "Where should we listen for incoming connections?")
	staticContentDir = flag.String("static-content-dir", "./", "A directory of static content to serve.")
	enableProfiling  = flag.Bool("enable-profiling", false, "")
	enableTracing    = flag.Bool("enable-tracing", false, "")
	enableMetrics    = flag.Bool("enable-metrics", false, "")
)

func main() {
	flag.Parse()
	log.Printf("listen: %q", *listen)
	sa, err := metadata.Email("")
	if err != nil {
		log.Fatalf("Error fetching service account: %v", err)
	}
	log.Printf("serviceaccount: %s", sa)

	// Cloud Profiler initialization, best done as early as possible.
	if *enableProfiling {
		if err := profiler.Start(profiler.Config{
			Service:        "webalator",
			ServiceVersion: "0.0.1",
		}); err != nil {
			log.Fatalf("Error initializing profiler: %v", err)
		}
	}

	// Create and register a OpenCensus Stackdriver Trace exporter.
	if *enableTracing {
		exporter, err := stackdriver.NewExporter(stackdriver.Options{})
		if err != nil {
			log.Fatal("Error initializing tracing: %v", err)
		}
		trace.RegisterExporter(exporter)
	}

	if *enableMetrics {
		exporter, err := stackdriver.NewExporter(stackdriver.Options{
			MetricPrefix:      "webalator",
			ReportingInterval: 60 * time.Second,
		})
		if err != nil {
			log.Fatal("Error initializing tracing: %v", err)
		}
		exporter.StartMetricsExporter()
		defer exporter.Flush()
		defer exporter.StopMetricsExporter()
	}

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Running from: %s", dir)

	site := site.New(*staticContentDir)

	serveMux := http.NewServeMux()
	serveMux.Handle("/", site.Mux)

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
