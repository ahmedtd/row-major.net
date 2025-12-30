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

	"row-major/medtracker/dblayer"
	"row-major/medtracker/webui"
	"row-major/webalator/healthz"

	"cloud.google.com/go/firestore"
)

var (
	debugListen         = flag.String("debug-listen", "127.0.0.1:8001", "Server address:port for debug endpoint.")
	uiListen            = flag.String("ui-listen", "127.0.0.1:8000", "Server address:port for ui endpoint.")
	dataProject         = flag.String("data-project", "", "GCP project that contains the application state.")
	googleOAuthClientID = flag.String("google-oauth-client-id", "", "Google OAuth Client ID for the application.  Used for Sign In With Google.")
)

func main() {
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// GCP Cloud Logging prefers "message"
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
		slog.String("ui-listen", *uiListen),
		slog.String("data-project", *dataProject),
		slog.String("google-oauth-client-id", *googleOAuthClientID),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := do(ctx); err != nil {
		slog.ErrorContext(ctx, "Error", slog.Any("err", err))
		os.Exit(255)
	}
}

func GCPCloudLoggingHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		slog.InfoContext(
			r.Context(),
			"Processed HTTP Request",
			slog.Group(
				"httpRequest",
				slog.String("requestMethod", r.Method),
				slog.String("requestURL", r.URL.String()),
			))
	})
}

func do(ctx context.Context) error {
	fstore, err := firestore.NewClient(ctx, *dataProject)
	if err != nil {
		return fmt.Errorf("while creating FireStore client: %w", err)
	}

	db := dblayer.New(fstore, *googleOAuthClientID)

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

	ui := webui.New(fstore, db, *googleOAuthClientID)
	uiServeMux := http.NewServeMux()
	uiServer := &http.Server{
		Addr:    *uiListen,
		Handler: GCPCloudLoggingHTTPMiddleware(uiServeMux),

		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	ui.Register(uiServeMux)

	go func() {
		if err := debugServer.ListenAndServe(); err != nil {
			slog.ErrorContext(ctx, "Debug server died", slog.Any("err", err))
			os.Exit(255)
		}
	}()

	go func() {
		if err := uiServer.ListenAndServe(); err != nil {
			slog.ErrorContext(ctx, "UI server died", slog.Any("err", err))
			os.Exit(255)
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	return nil
}
