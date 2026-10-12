package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"row-major/webalator/healthz"
	"row-major/webalator/imgalator"
	"row-major/webalator/mdredir"
	"row-major/webalator/proxyipreflect"
	"row-major/webalator/site"
	"row-major/webalator/sitecontent"
	"syscall"
	"time"

	"github.com/golang/glog"
)

var (
	listen      = flag.String("listen", "0.0.0.0:8080", "Where should we listen for incoming connections?")
	debugListen = flag.String("debug-listen", "0.0.0.0:8081", "Where should we listen for the debug interface?")

	imgalatorBucket = flag.String("imgalator-bucket", "", "Bucket to access using imgalator")
)

func main() {
	flag.Parse()

	glog.CopyStandardLogTo("INFO")

	glog.Infof("flags:")
	glog.Infof("listen: %v", *listen)
	glog.Infof("debug-listen: %v", *debugListen)

	glog.Infof("imgalator-bucket: %v", *imgalatorBucket)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir, err := os.Getwd()
	if err != nil {
		glog.Fatalf("Error getting current workind dir: %v", err)
	}
	glog.Infof("Running from: %s", dir)

	contentHandler, err := sitecontent.NewEmbeddedHandler()
	if err != nil {
		glog.Fatalf("Error while creating site content handler: %v", err)
	}

	site, err := site.New(contentHandler)
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

	server := &http.Server{
		Addr: *listen,

		Handler: serveMux,

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
