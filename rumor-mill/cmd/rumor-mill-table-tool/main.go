// rumor-mill-table-tool is a utility program for interacting with data stored
// in our GCS tables.
package main

import (
	"context"
	"flag"

	"row-major/rumor-mill/table"
	trackerpb "row-major/rumor-mill/table/trackerpb"

	"cloud.google.com/go/storage"
	"github.com/golang/glog"
	googleopt "google.golang.org/api/option"
)

var (
	dataProject = flag.String("data-project", "", "GCP project for cloud resources.")
	dataDir     = flag.String("data-dir", "", "GCS bucket for database")
)

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("INFO")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gcs, err := storage.NewClient(ctx, googleopt.WithGRPCConnectionPool(1))
	if err != nil {
		glog.Fatalf("Failed to create new GCS client: %v", err)
	}

	watchConfigTable := table.NewWatchConfigTable(gcs, *dataDir)

	wc := &trackerpb.WatchConfig{
		Id:              1,
		Description:     "Kubernetes and GKE Articles",
		TopicRegexp:     "kubernetes|k8s|gke|google ?kubernetes ?engine|google ?container ?engine|anthos|cloud ?run|kcc|config ?connector",
		NotifyAddresses: []string{"ahmed.taahir@gmail.com"},
	}

	if err := watchConfigTable.Create(ctx, wc); err != nil {
		glog.Fatalf("Failed to create WatchConfig: %v", err)
	}
}
