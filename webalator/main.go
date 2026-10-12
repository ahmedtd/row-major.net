package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"row-major/medtracker/dblayer"
	"row-major/medtracker/webui"
	"row-major/webalator/healthz"
	"row-major/webalator/mdredir"
	"row-major/webalator/proxyipreflect"
	"row-major/webalator/site"
	"row-major/webalator/sitecontent"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
)

var (
	listen      = flag.String("listen", "0.0.0.0:8080", "Where should we listen for incoming connections?")
	debugListen = flag.String("debug-listen", "0.0.0.0:8081", "Where should we listen for the debug interface?")

	medtrackerDataProject = flag.String("medtracker-data-project", "", "GCP project that contains the medtracker application state.")
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
		slog.String("medtracker-data-project", *medtrackerDataProject),
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

	medtrackerFirestore, err := firestore.NewClient(ctx, *medtrackerDataProject)
	if err != nil {
		slog.ErrorContext(ctx, "Error creating medtracker FireStore client", slog.Any("err", err))
		os.Exit(1)
	}
	medtrackerUI := webui.New(medtrackerFirestore, dblayer.New(medtrackerFirestore))

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

	serveMux := http.NewServeMux()
	serveMux.Handle("/", site.Mux)
	serveMux.Handle("/metadata-redirect", mdredir.New())
	serveMux.Handle("/proxy-ip-reflect", proxyipreflect.New())
	serveMux.Handle("/healthz", healthz.New())
	serveMux.Handle("/readyz", healthz.New())
	medtrackerUI.Register(serveMux, "medtracker.dev")

	server := &http.Server{
		Addr: *listen,

		Handler: GCPCloudLoggingHTTPMiddleware(serveMux),

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

// statusCatcher safely catches properties of the HTTP response.
//
// See https://www.alexedwards.net/blog/how-to-use-the-http-responsecontroller-type
type statusCatcher struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func newStatusCatcher(w http.ResponseWriter) *statusCatcher {
	return &statusCatcher{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (mw *statusCatcher) WriteHeader(statusCode int) {
	mw.ResponseWriter.WriteHeader(statusCode)

	if !mw.headerWritten {
		mw.statusCode = statusCode
		mw.headerWritten = true
	}
}

func (mw *statusCatcher) Write(b []byte) (int, error) {
	mw.headerWritten = true
	return mw.ResponseWriter.Write(b)
}

func (mw *statusCatcher) Unwrap() http.ResponseWriter {
	return mw.ResponseWriter
}

func GCPCloudLoggingHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := newStatusCatcher(w)

		next.ServeHTTP(sc, r)

		slog.InfoContext(
			r.Context(),
			"HTTP Request",

			// Cloud Logging has slightly special handling for "httpRequest".
			// Not sure if it actually adds anything over just putting a similar
			// struct in jsonPayload.
			//
			// https://docs.cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#httprequest
			slog.Group(
				"httpRequest",
				slog.String("requestMethod", r.Method),
				slog.String("requestUrl", r.URL.String()),
				slog.Int("status", sc.statusCode),
			),
		)
	})
}
