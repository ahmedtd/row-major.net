// rumor-mill-table-tool is a utility program for interacting with data stored
// in our GCS tables.
package main

import (
	"context"
	"fmt"

	"row-major/rumor-mill/table"
	trackerpb "row-major/rumor-mill/table/trackerpb"

	"cloud.google.com/go/storage"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"google.golang.org/api/iterator"
	googleopt "google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/prototext"
)

var cmdRoot = &cobra.Command{
	Use: "rumor-tool",
}

var (
	dataProject string
	dataDir     string
)

func init() {
	cmdRoot.PersistentFlags().StringVar(&dataProject, "data-project", "", "GCP project for cloud resources.")
	cmdRoot.PersistentFlags().StringVar(&dataDir, "data-dir", "", "GCS bucket for database")
}

var cmdWatchConfigs = &cobra.Command{
	Use: "watch-configs [command]",
}

var cmdWatchConfigsList = &cobra.Command{
	Use: "list",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gcs, err := storage.NewClient(ctx, googleopt.WithGRPCConnectionPool(1))
		if err != nil {
			return fmt.Errorf("while creating GCS client: %w", err)
		}

		watchConfigTable := table.NewWatchConfigTable(gcs, dataDir)

		it := watchConfigTable.List(ctx)
		for {
			wc, err := it.Next(ctx)
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("while advancing WatchConfig iterator: %w", err)
			}

			fmt.Println(prototext.Format(wc))
		}

		return nil
	},
}

var cmdWatchConfigsCreate = &cobra.Command{
	Use: "create",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gcs, err := storage.NewClient(ctx, googleopt.WithGRPCConnectionPool(1))
		if err != nil {
			return fmt.Errorf("while creating GCS client: %w", err)
		}

		wcTable := table.NewWatchConfigTable(gcs, dataDir)

		wc := &trackerpb.WatchConfig{
			Id:              watchConfigsCreateID,
			Description:     watchConfigsCreateDescription,
			TopicRegexp:     watchConfigsCreateTopicRegexp,
			NotifyAddresses: watchConfigsCreateNotifyAddresses,
		}

		if err := wcTable.Create(ctx, wc); err != nil {
			return fmt.Errorf("while creating WatchConfig: %w", err)
		}

		return nil
	},
}

var (
	watchConfigsCreateID              uint64
	watchConfigsCreateDescription     string
	watchConfigsCreateTopicRegexp     string
	watchConfigsCreateNotifyAddresses []string
)

func init() {
	cmdWatchConfigsCreate.Flags().Uint64Var(&watchConfigsCreateID, "id", 0, "")
	cmdWatchConfigsCreate.Flags().StringVar(&watchConfigsCreateDescription, "description", "", "")
	cmdWatchConfigsCreate.Flags().StringVar(&watchConfigsCreateTopicRegexp, "topic-regexp", "", "")
	cmdWatchConfigsCreate.Flags().StringSliceVar(&watchConfigsCreateNotifyAddresses, "notify-addresses", []string{}, "")
}

func main() {
	glog.CopyStandardLogTo("INFO")

	cmdRoot.AddCommand(cmdWatchConfigs)
	cmdWatchConfigs.AddCommand(cmdWatchConfigsCreate, cmdWatchConfigsList)

	cmdRoot.Execute()
}
