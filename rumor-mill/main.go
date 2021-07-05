// rumor_mill is a long-lived program that scrapes Hacker News for articles
// matching a regexp interest pattern and sends alerts based on its findings.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"row-major/rumor-mill/hackernews"
	"row-major/rumor-mill/scraper"
	"row-major/rumor-mill/table"
	"row-major/webalator/healthz"

	"cloud.google.com/go/storage"
	cloudmetrics "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	cloudtrace "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"github.com/golang/glog"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	googleopt "google.golang.org/api/option"
)

var (
	debugListen          = flag.String("debug-listen", "127.0.0.1:8001", "Server address:port for debug endpoint.")
	dataDir              = flag.String("data-dir", "", "GCS bucket for database")
	userAgent            = flag.String("user-agent", "row-major.net/rumor-mill", "User-Agent to use for all scraping operations.")
	monitoring           = flag.Bool("monitoring", false, "Enable monitoring?")
	monitoringProject    = flag.String("monitoring-project", "", "Override project used for monitoring integration.  If not specified, the project associated with Application Default Credentials is used.")
	monitoringTraceRatio = flag.Float64("monitoring-trace-ratio", 0.0001, "What ratio of traces should be exported?")
)

var (
	topicRegexp = regexp.MustCompile(`kubernetes|k8s|gke|google ?kubernetes ?engine|google ?container ?engine|anthos|cloud ?run|kcc|nomos`)
)

func main() {
	flag.Parse()

	glog.CopyStandardLogTo("INFO")

	glog.Infof("flags:")
	glog.Infof("debug-listen: %q", *debugListen)
	glog.Infof("data-dir: %q", *dataDir)
	glog.Infof("user-agent: %q", *userAgent)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *monitoring {
		metricsOpts := []cloudmetrics.Option{}
		traceOpts := []cloudtrace.Option{}
		if *monitoringProject != "" {
			metricsOpts = append(metricsOpts, cloudmetrics.WithProjectID(*monitoringProject))
			traceOpts = append(traceOpts, cloudtrace.WithProjectID(*monitoringProject))
		}

		_, traceShutdown, err := cloudtrace.InstallNewPipeline(traceOpts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(*monitoringTraceRatio)))
		if err != nil {
			glog.Fatalf("Failed to install Cloud Trace OpenTelemetry trace pipeline: %v", err)
		}
		defer traceShutdown()

		pusher, err := cloudmetrics.InstallNewPipeline(metricsOpts)
		if err != nil {
			glog.Fatalf("Failed to install Cloud Metrics OpenTelemetry meter pipeline: %v", err)
		}
		defer pusher.Stop(ctx)
	}

	httpClient := &http.Client{}
	hn := hackernews.New(httpClient, "hacker-news.firebaseio.com")

	debugServeMux := http.NewServeMux()
	debugServeMux.Handle("/healthz", healthz.New())
	debugServeMux.Handle("/readyz", healthz.New())
	debugServer := &http.Server{
		Addr:    *debugListen,
		Handler: debugServeMux,

		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	gcs, err := storage.NewClient(ctx, googleopt.WithGRPCConnectionPool(1))
	if err != nil {
		glog.Fatalf("Failed to create new GCS client: %v", err)
	}

	trackedArticles := table.NewTrackedArticleTable(gcs, *dataDir)

	scr := scraper.New(
		hn,
		trackedArticles,
		scraper.WithWatchConfig(&scraper.WatchConfig{
			ID:              1,
			TopicRegexp:     topicRegexp,
			NotifyAddresses: []string{},
		}),
	)
	scr.RegisterDebugHandlers(debugServeMux)

	go func() {
		if err := debugServer.ListenAndServe(); err != nil {
			glog.Fatalf("Debug server died: %v", err)
		}
	}()

	go func() {
		scr.Run(ctx)
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	glog.Flush()
}
