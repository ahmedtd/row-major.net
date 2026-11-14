// Package metadatasync walks every collection's Cloud Storage location and
// makes sure each object has an up-to-date ObjectMetadata entry.  A sync is
// expensive (one Firestore read per object), so it only runs when explicitly
// triggered.
package metadatasync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
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

	// running is set while a sync started by Start is in progress.
	running atomic.Bool
}

func New(db *dblayer.DB, storageClient *storage.Client) *Syncer {
	return &Syncer{
		db:            db,
		storageClient: storageClient,
	}
}

// Start begins syncing all collections on a background goroutine and returns
// immediately.  If a sync is already in progress, it does nothing and returns
// false.
//
// The sync is not tied to any caller's context (such as an HTTP request), so
// it keeps going after the caller returns.
func (s *Syncer) Start() bool {
	if !s.running.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer s.running.Store(false)
		s.SyncAll(context.Background())
	}()
	return true
}

// Running reports whether a sync started by Start is in progress.
func (s *Syncer) Running() bool {
	return s.running.Load()
}

// SyncAll makes one pass over every collection, blocking until it is done.
// Errors are logged rather than returned, so that one bad collection doesn't
// stop the others from being synced.
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

		scanned, created, updated, err := s.SyncCollection(ctx, collection)
		if err != nil {
			slog.ErrorContext(ctx, "Error while syncing collection object metadata",
				slog.String("collection", collection.ID),
				slog.Int("scanned", scanned),
				slog.Int("created", created),
				slog.Int("updated", updated),
				slog.Any("err", err))
			continue
		}
		slog.InfoContext(ctx, "Synced collection object metadata",
			slog.String("collection", collection.ID),
			slog.Int("scanned", scanned),
			slog.Int("created", created),
			slog.Int("updated", updated))
	}

	slog.InfoContext(ctx, "Finished artboard object metadata sync",
		slog.Int("collections", len(collections)),
		slog.Duration("elapsed", time.Since(start)))
}

// SyncCollection makes sure every object in the collection's storage location
// has an up-to-date metadata entry (see dblayer.EnsureObjectMetadata).  It
// returns the number of objects scanned, the number of entries created, and
// the number of existing entries updated.
func (s *Syncer) SyncCollection(ctx context.Context, collection *dbtypes.Collection) (scanned, created, updated int, err error) {
	loc, err := gcsurl.Parse(collection.StorageURL)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("while parsing storage URL %q: %w", collection.StorageURL, err)
	}

	flush := func(paths []string) error {
		c, u, err := s.db.EnsureObjectMetadata(ctx, collection.ID, paths)
		created += c
		updated += u
		return err
	}

	query := &storage.Query{Prefix: loc.Prefix}
	// Only names are needed.
	if err := query.SetAttrSelection([]string{"Name"}); err != nil {
		return 0, 0, 0, fmt.Errorf("while configuring object listing: %w", err)
	}
	it := s.storageClient.Bucket(loc.Bucket).Objects(ctx, query)

	batch := make([]string, 0, batchSize)
	for {
		obj, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return scanned, created, updated, fmt.Errorf("while listing objects in %s: %w", loc, err)
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
				return scanned, created, updated, err
			}
			batch = batch[:0]
		}
	}
	if err := flush(batch); err != nil {
		return scanned, created, updated, err
	}

	return scanned, created, updated, nil
}
