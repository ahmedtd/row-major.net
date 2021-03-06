package main

import (
	"archive/zip"
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"row-major/webalator/contentpack"
	"row-major/webalator/healthz"
	"row-major/webalator/httpmetrics"
	"row-major/webalator/imgalator"
	"row-major/webalator/mdredir"
	"row-major/webalator/proxyipreflect"
	"row-major/webalator/site"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/profiler"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/monitoredresource"
	cloudtrace "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"github.com/golang/glog"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	listen      = flag.String("listen", "0.0.0.0:8080", "Where should we listen for incoming connections?")
	debugListen = flag.String("debug-listen", "0.0.0.0:8081", "Where should we listen for the debug interface?")
	contentPack = flag.String("content-pack", "", "URL of the content pack to serve.")

	enableProfiling = flag.Bool("enable-profiling", false, "")
	enableTracing   = flag.Bool("enable-tracing", false, "")
	tracingRatio    = flag.Float64("tracing-ratio", 0.001, "")
	enableMetrics   = flag.Bool("enable-metrics", false, "")

	imgalatorBucket = flag.String("imgalator-bucket", "", "Bucket to access using imgalator")
)

func main() {
	flag.Parse()

	glog.CopyStandardLogTo("INFO")

	glog.Infof("flags:")
	glog.Infof("listen: %v", *listen)
	glog.Infof("debug-listen: %v", *debugListen)
	glog.Infof("content-pack: %v", *contentPack)
	glog.Infof("enable-profiling: %v", *enableProfiling)
	glog.Infof("enable-tracing: %v", *enableTracing)
	glog.Infof("tracing-ratio: %v", *tracingRatio)
	glog.Infof("enable-metrics: %v", *enableMetrics)

	glog.Infof("imgalator-bucket: %v", *imgalatorBucket)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cloud Profiler initialization, best done as early as possible.
	if *enableProfiling {
		if err := profiler.Start(profiler.Config{
			Service:        "webalator",
			ServiceVersion: "0.0.1",
		}); err != nil {
			glog.Fatalf("Error initializing profiler: %v", err)
		}
	}

	// Create and register a OpenCensus Stackdriver Trace exporter.
	if *enableTracing {
		_, traceShutdown, err := cloudtrace.InstallNewPipeline(
			nil,
			sdktrace.WithSampler(sdktrace.TraceIDRatioBased(*tracingRatio)),
		)
		if err != nil {
			glog.Fatalf("Failed to install Cloud Trace OpenTelemetry trace pipeline: %v", err)
		}
		defer traceShutdown()
	}

	if *enableMetrics {
		exporter, err := stackdriver.NewExporter(stackdriver.Options{
			MetricPrefix:      "webalator",
			ReportingInterval: 60 * time.Second,
			MonitoredResource: monitoredresource.Autodetect(),
		})
		if err != nil {
			glog.Fatalf("Error initializing tracing: %v", err)
		}
		exporter.StartMetricsExporter()
		defer exporter.Flush()
		defer exporter.StopMetricsExporter()
	}

	dir, err := os.Getwd()
	if err != nil {
		glog.Fatalf("Error getting current workind dir: %v", err)
	}
	glog.Infof("Running from: %s", dir)

	var contentPackReader *zip.ReadCloser
	if strings.HasPrefix(*contentPack, "file://") {
		filePath := strings.TrimPrefix(*contentPack, "file://")
		reader, err := zip.OpenReader(filePath)
		if err != nil {
			glog.Fatalf("Error opening content pack: %v", err)
		}
		contentPackReader = reader
	} else {
		glog.Fatalf("Unsupported content pack: %v", *contentPack)
	}
	defer contentPackReader.Close()

	contentPackHandler, err := contentpack.NewHandler(&contentPackReader.Reader)
	if err != nil {
		glog.Fatalf("Error while creating content pack handler: %v", err)
	}

	site, err := site.New(contentPackHandler)
	if err != nil {
		glog.Fatalf("Error creating site: %v", err)
	}

	imgalator, err := imgalator.New(context.Background(), "/imgalator", *imgalatorBucket)
	if err != nil {
		glog.Fatalf("Error creating imgalator: %v", err)
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

	serveMux := http.NewServeMux()
	serveMux.Handle("/", site.Mux)
	serveMux.Handle("/imgalator/", imgalator)
	serveMux.Handle("/metadata-redirect", mdredir.New())
	serveMux.Handle("/proxy-ip-reflect", proxyipreflect.New())
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

	go func() {
		if err := debugServer.ListenAndServe(); err != nil {
			glog.Fatalf("Debug server died: %v", err)
		}
	}()

	go func() {
		if err := server.ListenAndServe(); err != nil {
			glog.Fatalf("Error while serving http: %v", err)
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	glog.Flush()
}
