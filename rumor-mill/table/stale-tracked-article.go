package table

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"strconv"
	"strings"

	trackerpb "row-major/rumor-mill/table/trackerpb"

	"cloud.google.com/go/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

const staleTrackedArticleKeyPrefix = "tables/stale-hackernews-tracked-articles/"

// StaleTrackedArticleTable fronts the stale-hackernews-tracked-articles table
// in GCS.
//
// We sweep old articles into this table to minimize the number of entries that
// the scraper has to process during the alert join.
type StaleTrackedArticleTable struct {
	gcs    *storage.Client
	bucket string
}

func NewStaleTrackedArticleTable(gcs *storage.Client, bucket string) *StaleTrackedArticleTable {
	return &StaleTrackedArticleTable{
		gcs:    gcs,
		bucket: bucket,
	}
}

func (t *StaleTrackedArticleTable) gcsPathForID(id uint64) string {
	return path.Join(staleTrackedArticleKeyPrefix, strconv.FormatUint(id, 10))
}

func (t *StaleTrackedArticleTable) idFromGCSName(name string) (uint64, error) {
	return strconv.ParseUint(strings.TrimPrefix(name, staleTrackedArticleKeyPrefix), 10, 64)
}

// Get gets the TrackedArticle with the given ID from GCS.
//
// Returns the TrackedArticle, a "found" indicator, and an error.
func (t *StaleTrackedArticleTable) Get(ctx context.Context, id uint64) (*trackerpb.TrackedArticle, bool, error) {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "StaleTrackedArticleTable.Get")
	defer span.End()

	span.SetAttributes(attribute.Int64("id", int64(id)))

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(id))

	r, err := obj.NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			span.SetStatus(codes.Ok, "")
			return nil, false, nil
		}

		err := fmt.Errorf("while opening reader for object: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		err := fmt.Errorf("while reading from object: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	ta := &trackerpb.TrackedArticle{}
	if err := proto.Unmarshal(data, ta); err != nil {
		err := fmt.Errorf("while unmarshaling TrackedArticle proto: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	if id != ta.Id {
		err := fmt.Errorf("ID mismatch in TrackedArticle")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	ta.Generation = r.Attrs.Generation
	ta.Metageneration = r.Attrs.Metageneration

	span.SetStatus(codes.Ok, "")

	return ta, true, nil
}

func (t *StaleTrackedArticleTable) Create(ctx context.Context, in *trackerpb.TrackedArticle) error {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "StaleTrackedArticleTable.Create")
	defer span.End()

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(in.Id))

	// Make sure that the GCS-specific metadata is zeroed out before writing the
	// object back to storage.
	clone := proto.Clone(in).(*trackerpb.TrackedArticle)
	clone.Generation = 0
	clone.Metageneration = 0

	data, err := proto.Marshal(clone)
	if err != nil {
		return fmt.Errorf("while marshaling TrackedArticle proto: %w", err)
	}

	// Create condition: object does not currently exist.
	w := obj.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)

	// Disable chunking.  This will expose more transient server errors to
	// calling code, but significantly reduces memory usage.
	w.ChunkSize = 0

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("while writing TrackedArticle to object writer: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("while closing object writer: %w", err)
	}

	// Update with result of successful write.
	in.Generation = w.Attrs().Generation
	in.Metageneration = w.Attrs().Metageneration

	return nil
}

func (t *StaleTrackedArticleTable) Update(ctx context.Context, in *trackerpb.TrackedArticle) error {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "StaleTrackedArticleTable.Update")
	defer span.End()

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(in.Id))

	// Make sure that the GCS-specific metadata is zeroed out before writing the
	// object back to storage.
	clone := proto.Clone(in).(*trackerpb.TrackedArticle)
	clone.Generation = 0
	clone.Metageneration = 0

	data, err := proto.Marshal(clone)
	if err != nil {
		return fmt.Errorf("while marshaling TrackedArticle proto: %w", err)
	}

	// Update condition: object exists at the generation we're working from.
	w := obj.If(storage.Conditions{GenerationMatch: in.Generation, MetagenerationMatch: in.Metageneration}).NewWriter(ctx)

	// Disable chunking.  This will expose more transient server errors to
	// calling code, but significantly reduces memory usage.
	w.ChunkSize = 0

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("while writing TrackedArticle to object writer: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("while closing object writer: %w", err)
	}

	// Update with result of successful write.
	in.Generation = w.Attrs().Generation
	in.Metageneration = w.Attrs().Metageneration

	return nil
}

func (t *StaleTrackedArticleTable) Delete(ctx context.Context, in *trackerpb.TrackedArticle) error {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "StaleTrackedArticleTable.Delete")
	defer span.End()

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(in.Id))

	// Delete condition: object exists at the generation we're working from.
	cond := storage.Conditions{GenerationMatch: in.Generation, MetagenerationMatch: in.Metageneration}
	if err := obj.If(cond).Delete(ctx); err != nil {
		return fmt.Errorf("while deleting object: %w", err)
	}

	return nil
}
