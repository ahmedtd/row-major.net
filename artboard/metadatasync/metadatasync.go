// Package metadatasync periodically walks every collection's Cloud Storage
// location and makes sure each object has an ObjectMetadata entry.
package metadatasync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"row-major/artboard/dblayer"
	"row-major/artboard/dbtypes"
	"row-major/artboard/gcsurl"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// batchSize is the number of objects whose metadata is checked in one
// Firestore round trip.
const batchSize = 300

type Syncer struct {
	db *dblayer.DB

	// storageClient accesses collection buckets using the server's ambient
	// (Workload Identity) credentials.
	storageClient *storage.Client
}

func New(db *dblayer.DB, storageClient *storage.Client) *Syncer {
	return &Syncer{
		db:            db,
		storageClient: storageClient,
	}
}

// Run syncs all collections immediately, and then again every interval, until
// ctx is cancelled.  Errors are logged rather than returned, so that one bad
// collection or a transient failure doesn't stop the loop.
func (s *Syncer) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		s.SyncAll(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SyncAll makes one pass over every collection.
func (s *Syncer) SyncAll(ctx context.Context) {
	start := time.Now()
	slog.InfoContext(ctx, "Starting artboard object metadata sync")

	collections, err := s.db.ListAllCollections(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Error while listing collections for metadata sync", slog.Any("err", err))
		return
	}

	for _, collection := range collections {
		if ctx.Err() != nil {
			return
		}

		scanned, created, err := s.SyncCollection(ctx, collection)
		if err != nil {
			slog.ErrorContext(ctx, "Error while syncing collection object metadata",
				slog.String("collection", collection.ID),
				slog.Int("scanned", scanned),
				slog.Int("created", created),
				slog.Any("err", err))
			continue
		}
		slog.InfoContext(ctx, "Synced collection object metadata",
			slog.String("collection", collection.ID),
			slog.Int("scanned", scanned),
			slog.Int("created", created))
	}

	slog.InfoContext(ctx, "Finished artboard object metadata sync",
		slog.Int("collections", len(collections)),
		slog.Duration("elapsed", time.Since(start)))
}

// SyncCollection makes sure every object in the collection's storage location
// has a metadata entry.  It returns the number of objects scanned and the
// number of entries created.
func (s *Syncer) SyncCollection(ctx context.Context, collection *dbtypes.Collection) (scanned, created int, err error) {
	loc, err := gcsurl.Parse(collection.StorageURL)
	if err != nil {
		return 0, 0, fmt.Errorf("while parsing storage URL %q: %w", collection.StorageURL, err)
	}

	flush := func(paths []string) error {
		n, err := s.db.EnsureObjectMetadata(ctx, collection.ID, paths)
		created += n
		return err
	}

	query := &storage.Query{Prefix: loc.Prefix}
	// Only names are needed.
	if err := query.SetAttrSelection([]string{"Name"}); err != nil {
		return 0, 0, fmt.Errorf("while configuring object listing: %w", err)
	}
	it := s.storageClient.Bucket(loc.Bucket).Objects(ctx, query)

	batch := make([]string, 0, batchSize)
	for {
		obj, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return scanned, created, fmt.Errorf("while listing objects in %s: %w", loc, err)
		}

		path := strings.TrimPrefix(obj.Name, loc.Prefix)
		if path == "" || strings.HasSuffix(path, "/") {
			// Skip folder placeholder objects.
			continue
		}
		scanned++

		batch = append(batch, path)
		if len(batch) == batchSize {
			if err := flush(batch); err != nil {
				return scanned, created, err
			}
			batch = batch[:0]
		}
	}
	if err := flush(batch); err != nil {
		return scanned, created, err
	}

	return scanned, created, nil
}
