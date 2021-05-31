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
	"row-major/webalator/healthz"

	"github.com/golang/glog"
)

var (
	debugListen = flag.String("debug-listen", "127.0.0.1:8001", "Server address:port for debug endpoint.")
	stateDir    = flag.String("state-dir", "", "GCS prefix for holding state.")
	userAgent   = flag.String("user-agent", "row-major.net/rumor-mill", "User-Agent to use for all scraping operations.")
)

var (
	topicRegexp = regexp.MustCompile(`kubernetes|k8s|gke|google ?kubernetes ?engine|google ?container ?engine|anthos|cloud ?run|kcc|nomos`)
)

func main() {
	flag.Parse()

	glog.CopyStandardLogTo("INFO")

	glog.Infof("flags:")
	glog.Infof("debug-listen: %q", *debugListen)
	glog.Infof("state-dir: %q", *stateDir)
	glog.Infof("user-agent: %q", *userAgent)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	scr := scraper.New(
		hn,
		scraper.WithWatchConfig(&scraper.WatchConfig{
			ID:          1,
			TopicRegexp: topicRegexp,
			NotifyAddresses: []string{
				"rumor-mill-gke@google.com",
				// Since the emails come from me, Gmail doesn't display them to me, even
				// though I am subscribed to rumor-mill-gke.
				"taahm@google.com",
			},
		}),
		scraper.WithGCSCheckpointFile(*stateDir),
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
