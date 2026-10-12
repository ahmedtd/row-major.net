// rumor_mill is a long-lived program that scrapes Hacker News for articles
// matching a regexp interest pattern and sends alerts based on its findings.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"row-major/rumor-mill/hackernews"
	"row-major/rumor-mill/scraper"
	"row-major/webalator/healthz"

	"cloud.google.com/go/firestore"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	cloudmetrics "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	cloudtrace "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"github.com/sendgrid/sendgrid-go"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

var (
	debugListen  = flag.String("debug-listen", "127.0.0.1:8001", "Server address:port for debug endpoint.")
	userAgent    = flag.String("user-agent", "row-major.net/rumor-mill", "User-Agent to use for all scraping operations.")
	scrapePeriod = flag.Duration("scrape-period", 30*time.Minute, "Time between scraper passes.")

	dataProject       = flag.String("data-project", "", "GCP project for cloud resources.")
	sendgridKeySecret = flag.String("sendgrid-key-secret", "sendgrid-api-key", "GCP Secret Manager secret name containing SendGrid API key.")

	monitoring           = flag.Bool("monitoring", false, "Enable monitoring?")
	monitoringProject    = flag.String("monitoring-project", "", "Override project used for monitoring integration.  If not specified, the project associated with Application Default Credentials is used.")
	monitoringTraceRatio = flag.Float64("monitoring-trace-ratio", 0.0001, "What ratio of traces should be exported?")
)

func main() {
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// GCP Cloud Logging gives "message" nicer handling in the overview pane.
			if a.Key == "msg" {
				a.Key = "message"
			}
			return a
		},
	}))
	slog.SetDefault(logger)

	slog.Info("Starting up")
	slog.Info(
		"Flags",
		slog.String("debug-listen", *debugListen),
		slog.String("user-agent", *userAgent),
		slog.Duration("scrape-period", *scrapePeriod),
		slog.String("data-project", *dataProject),
		slog.String("sendgrid-key-secret", *sendgridKeySecret),
		slog.Bool("monitoring", *monitoring),
		slog.String("monitoring-project", *monitoringProject),
		slog.Float64("monitoring-trace-ratio", *monitoringTraceRatio),
	)

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
			slog.ErrorContext(ctx, "Failed to install Cloud Trace OpenTelemetry trace pipeline", slog.Any("err", err))
			os.Exit(255)
		}
		defer traceShutdown()

		pusher, err := cloudmetrics.InstallNewPipeline(metricsOpts)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to install Cloud Metrics OpenTelemetry meter pipeline", slog.Any("err", err))
			os.Exit(255)
		}
		defer pusher.Stop(ctx)
	}

	httpClient := &http.Client{}
	hn := hackernews.New(httpClient, "hacker-news.firebaseio.com")

	debugServeMux := http.NewServeMux()
	debugServeMux.Handle("/healthz", healthz.New())
	debugServeMux.Handle("/readyz", healthz.New())
	debugServeMux.HandleFunc("/debug/pprof/", pprof.Index)
	debugServeMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	debugServeMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	debugServeMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	debugServeMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	debugServer := &http.Server{
		Addr:    *debugListen,
		Handler: debugServeMux,

		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	sg, err := newSendgridClient(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create Sendgrid client", slog.Any("err", err))
		os.Exit(255)
	}

	fstore, err := firestore.NewClient(ctx, *dataProject)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create FireStore client", slog.Any("err", err))
		os.Exit(255)
	}

	scr := scraper.New(
		hn,
		sg,
		fstore,
		scraper.WithScrapePeriod(*scrapePeriod),
	)
	scr.RegisterDebugHandlers(debugServeMux)

	go func() {
		if err := debugServer.ListenAndServe(); err != nil {
			slog.ErrorContext(ctx, "Debug server died", slog.Any("err", err))
			os.Exit(255)
		}
	}()

	go func() {
		scr.Run(ctx)
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh
}

func newSendgridClient(ctx context.Context) (*sendgrid.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	secretClient, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("while creating Secret Manager client: %w", err)
	}
	defer secretClient.Close()

	resp, err := secretClient.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", *dataProject, *sendgridKeySecret),
	})
	if err != nil {
		return nil, fmt.Errorf("while pulling secret: %w", err)
	}

	return sendgrid.NewSendClient(string(resp.GetPayload().GetData())), nil
}
