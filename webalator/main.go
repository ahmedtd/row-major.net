package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"row-major/webalator/healthz"
	"row-major/webalator/mdredir"
	"row-major/webalator/proxyipreflect"
	"row-major/webalator/site"
	"row-major/webalator/sitecontent"
	"syscall"
	"time"
)

var (
	listen      = flag.String("listen", "0.0.0.0:8080", "Where should we listen for incoming connections?")
	debugListen = flag.String("debug-listen", "0.0.0.0:8081", "Where should we listen for the debug interface?")
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
		slog.String("listen", *listen),
		slog.String("debug-listen", *debugListen),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir, err := os.Getwd()
	if err != nil {
		slog.ErrorContext(ctx, "Error getting current working dir", slog.Any("err", err))
		os.Exit(1)
	}
	slog.InfoContext(ctx, "Running from", slog.String("dir", dir))

	contentHandler, err := sitecontent.NewEmbeddedHandler()
	if err != nil {
		slog.ErrorContext(ctx, "Error while creating site content handler", slog.Any("err", err))
		os.Exit(1)
	}

	site, err := site.New(contentHandler)
	if err != nil {
		slog.ErrorContext(ctx, "Error creating site", slog.Any("err", err))
		os.Exit(1)
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
	serveMux.Handle("/metadata-redirect", mdredir.New())
	serveMux.Handle("/proxy-ip-reflect", proxyipreflect.New())
	serveMux.Handle("/healthz", healthz.New())
	serveMux.Handle("/readyz", healthz.New())

	server := &http.Server{
		Addr: *listen,

		Handler: serveMux,

		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		if err := debugServer.ListenAndServe(); err != nil {
			slog.ErrorContext(ctx, "Debug server died", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	go func() {
		if err := server.ListenAndServe(); err != nil {
			slog.ErrorContext(ctx, "Error while serving http", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh
}
