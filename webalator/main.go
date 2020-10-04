package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"row-major/webalator/healthz"
	"row-major/webalator/httpmetrics"
	"row-major/webalator/mdredir"
	"row-major/webalator/site"
	"time"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/profiler"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/monitoredresource"
	"go.opencensus.io/trace"
)

var (
	listen                = flag.String("listen", "0.0.0.0:8080", "Where should we listen for incoming connections?")
	debugListen           = flag.String("debug-listen", "0.0.0.0:8081", "Where should we listen for the debug interface?")
	staticContentDir      = flag.String("static-content-dir", "./", "A directory of static content to serve.")
	templateDir           = flag.String("template-dir", "./", "A directory of templates to serve.")
	enableTemplateRefresh = flag.Bool("enable-template-refresh", false, "Should we refresh templates from disk?")
	enableProfiling       = flag.Bool("enable-profiling", false, "")
	enableTracing         = flag.Bool("enable-tracing", false, "")
	enableMetrics         = flag.Bool("enable-metrics", false, "")
)

func main() {
	flag.Parse()

	log.Printf("flags:")
	log.Printf("listen: %q", *listen)
	log.Printf("debug-listen: %q", *debugListen)
	log.Printf("static-content-dir: %q", *staticContentDir)
	log.Printf("template-dir: %q", *templateDir)
	log.Printf("enable-template-refresh: %q", *enableTemplateRefresh)
	log.Printf("enable-profiling: %q", *enableProfiling)
	log.Printf("enable-tracing: %q", *enableTracing)
	log.Printf("enable-metrics: %q", *enableMetrics)

	if metadata.OnGCE() {
		sa, err := metadata.Email("")
		if err != nil {
			log.Fatalf("Error fetching service account: %v", err)
		}
		log.Printf("serviceaccount: %s", sa)
	}

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
		exporter, err := stackdriver.NewExporter(stackdriver.Options{
			MonitoredResource: monitoredresource.Autodetect(),
		})
		if err != nil {
			log.Fatal("Error initializing tracing: %v", err)
		}
		trace.RegisterExporter(exporter)
	}

	if *enableMetrics {
		exporter, err := stackdriver.NewExporter(stackdriver.Options{
			MetricPrefix:      "webalator",
			ReportingInterval: 60 * time.Second,
			MonitoredResource: monitoredresource.Autodetect(),
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

	site, err := site.New(*staticContentDir, *templateDir, *enableTemplateRefresh)
	if err != nil {
		log.Fatalf("Error creating site: %v", err)
	}

	debugServeMux := http.NewServeMux()
	debugServeMux.Handle("/healthz", healthz.New())
	debugServer := &http.Server{
		Addr:    *debugListen,
		Handler: debugServeMux,

		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	go func() {
		if err := debugServer.ListenAndServe(); err != nil {
			log.Printf("Debug server died: %v", err)
		}
	}()

	serveMux := http.NewServeMux()
	serveMux.Handle("/", site.Mux)
	serveMux.Handle("/metadata-redirect", mdredir.New())
	serveMux.Handle("/healthz", healthz.New())
	serveMux.Handle("/readyz", healthz.New())

	mw := httpmetrics.New(serveMux)
	mw.RegisterMetrics()

	server := &http.Server{
		Addr: *listen,

		Handler: mw,

		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error while serving http: %v", err)
	}
}
