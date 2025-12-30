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

	"row-major/medtracker/poller"
	"row-major/webalator/healthz"

	"cloud.google.com/go/firestore"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/sendgrid/sendgrid-go"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

var (
	debugListen       = flag.String("debug-listen", "127.0.0.1:8001", "Server address:port for debug endpoint.")
	recheckPeriod     = flag.Duration("recheck-period", 1*time.Hour, "Time between scans")
	dataProject       = flag.String("data-project", "", "GCP project that contains the application state.")
	sendgridKeySecret = flag.String("sendgrid-key-secret", "", "GCP Secret Manager secret name that contains the Sendgrid API key")
)

func main() {
	flag.Parse()

	slog.Info("Starting up")
	slog.Info(
		"Flags",
		slog.String("debug-listen", *debugListen),
		slog.Duration("recheck-period", *recheckPeriod),
		slog.String("data-project", *dataProject),
		slog.String("sendgrid-key-secret", *sendgridKeySecret),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := do(ctx); err != nil {
		slog.ErrorContext(ctx, "Error", slog.Any("err", err))
		os.Exit(255)
	}
}

func do(ctx context.Context) error {
	sg, err := newSendgridClient(ctx)
	if err != nil {
		return fmt.Errorf("while creating Sendgrid client: %w", err)
	}

	fstore, err := firestore.NewClient(ctx, *dataProject)
	if err != nil {
		return fmt.Errorf("while creating FireStore client: %w", err)
	}

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

	poller := poller.New(fstore, sg, *recheckPeriod)

	go func() {
		if err := debugServer.ListenAndServe(); err != nil {
			slog.ErrorContext(ctx, "Debug server died", slog.Any("err", err))
			os.Exit(255)
		}
	}()

	go func() {
		poller.Run(ctx)
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	return nil
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
