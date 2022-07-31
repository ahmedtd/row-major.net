package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"row-major/medtracker/webui"
	"row-major/webalator/healthz"

	"cloud.google.com/go/firestore"
	"github.com/golang/glog"
)

var (
	debugListen = flag.String("debug-listen", "127.0.0.1:8001", "Server address:port for debug endpoint.")
	uiListen    = flag.String("ui-listen", "127.0.0.1:8000", "Server address:port for ui endpoint.")
	dataProject = flag.String("data-project", "", "GCP project that contains the application state.")
)

func main() {
	flag.Parse()

	glog.Infof("flags:")
	glog.Infof("debug-listen: %v", *debugListen)
	glog.Infof("ui-listen: %v", *uiListen)
	glog.Infof("data-project: %v", *dataProject)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := do(ctx); err != nil {
		glog.Exitf("Error: %v", err)
	}
}

func do(ctx context.Context) error {
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

	ui := webui.New(fstore)
	uiServeMux := http.NewServeMux()
	uiServer := &http.Server{
		Addr:    *uiListen,
		Handler: uiServeMux,

		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	ui.Register(uiServeMux)

	go func() {
		if err := debugServer.ListenAndServe(); err != nil {
			glog.Fatalf("Debug server died: %v", err)
		}
	}()

	go func() {
		if err := uiServer.ListenAndServe(); err != nil {
			glog.Fatalf("UI server died: %v", err)
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	glog.Flush()

	return nil
}
