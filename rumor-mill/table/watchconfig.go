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

const watchConfigKeyPrefix = "tables/watchconfigs/"

// WatchConfigTable fronts the watchconfig table in GCS.
type WatchConfigTable struct {
	gcs    *storage.Client
	bucket string
}

func NewWatchConfigTable(gcs *storage.Client, bucket string) *WatchConfigTable {
	return &WatchConfigTable{
		gcs:    gcs,
		bucket: bucket,
	}
}

func (t *WatchConfigTable) gcsPathForID(id uint64) string {
	return path.Join(watchConfigKeyPrefix, strconv.FormatUint(id, 10))
}

func (t *WatchConfigTable) idFromGCSName(name string) (uint64, error) {
	return strconv.ParseUint(strings.TrimPrefix(name, watchConfigKeyPrefix), 10, 64)
}

// Get gets the WatchConfig with the given ID from GCS.
//
// Returns the WatchConfig, a "found" indicator, and an error.
func (t *WatchConfigTable) Get(ctx context.Context, id uint64) (*trackerpb.WatchConfig, bool, error) {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "WatchConfigTable.Get")
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

	ta := &trackerpb.WatchConfig{}
	if err := proto.Unmarshal(data, ta); err != nil {
		err := fmt.Errorf("while unmarshaling WatchConfig proto: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	if id != ta.Id {
		err := fmt.Errorf("ID mismatch in WatchConfig")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	ta.Generation = r.Attrs.Generation
	ta.Metageneration = r.Attrs.Metageneration

	span.SetStatus(codes.Ok, "")

	return ta, true, nil
}

func (t *WatchConfigTable) Create(ctx context.Context, in *trackerpb.WatchConfig) error {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "WatchConfigTable.Create")
	defer span.End()

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(in.Id))

	// Make sure that the GCS-specific metadata is zeroed out before writing the
	// object back to storage.
	clone := proto.Clone(in).(*trackerpb.WatchConfig)
	clone.Generation = 0
	clone.Metageneration = 0

	data, err := proto.Marshal(clone)
	if err != nil {
		return fmt.Errorf("while marshaling WatchConfig proto: %w", err)
	}

	// Create condition: object does not currently exist.
	w := obj.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)

	// Disable chunking.  This will expose more transient server errors to
	// calling code, but significantly reduces memory usage.
	w.ChunkSize = 0

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("while writing WatchConfig to object writer: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("while closing object writer: %w", err)
	}

	// Update with result of successful write.
	in.Generation = w.Attrs().Generation
	in.Metageneration = w.Attrs().Metageneration

	return nil
}

func (t *WatchConfigTable) Update(ctx context.Context, in *trackerpb.WatchConfig) error {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "WatchConfigTable.Update")
	defer span.End()

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(in.Id))

	// Make sure that the GCS-specific metadata is zeroed out before writing the
	// object back to storage.
	clone := proto.Clone(in).(*trackerpb.WatchConfig)
	clone.Generation = 0
	clone.Metageneration = 0

	data, err := proto.Marshal(clone)
	if err != nil {
		return fmt.Errorf("while marshaling WatchConfig proto: %w", err)
	}

	// Update condition: object exists at the generation we're working from.
	w := obj.If(storage.Conditions{GenerationMatch: in.Generation, MetagenerationMatch: in.Metageneration}).NewWriter(ctx)

	// Disable chunking.  This will expose more transient server errors to
	// calling code, but significantly reduces memory usage.
	w.ChunkSize = 0

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("while writing WatchConfig to object writer: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("while closing object writer: %w", err)
	}

	// Update with result of successful write.
	in.Generation = w.Attrs().Generation
	in.Metageneration = w.Attrs().Metageneration

	return nil
}

func (t *WatchConfigTable) Delete(ctx context.Context, in *trackerpb.WatchConfig) error {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "WatchConfigTable.Delete")
	defer span.End()

	obj := t.gcs.Bucket(t.bucket).Object(t.gcsPathForID(in.Id))

	// Delete condition: object exists at the generation we're working from.
	cond := storage.Conditions{GenerationMatch: in.Generation, MetagenerationMatch: in.Metageneration}
	if err := obj.If(cond).Delete(ctx); err != nil {
		return fmt.Errorf("while deleting object: %w", err)
	}

	return nil
}

type WatchConfigIterator struct {
	table *WatchConfigTable
	inner *storage.ObjectIterator
}

func (it *WatchConfigIterator) Next(ctx context.Context) (*trackerpb.WatchConfig, error) {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "WatchConfigIterator.Next")
	defer span.End()

	for {
		attr, err := it.inner.Next()
		if err != nil {
			return nil, err
		}

		id, err := it.table.idFromGCSName(attr.Name)
		if err != nil {
			return nil, fmt.Errorf("while parsing ID: %w", err)
		}

		ta, ok, err := it.table.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("while reading watchconfig: %w", err)
		}

		if !ok {
			// Object was deleted during list.
			continue
		}

		return ta, nil
	}
}

func (t *WatchConfigTable) List(ctx context.Context) *WatchConfigIterator {
	return &WatchConfigIterator{
		table: t,
		inner: t.gcs.Bucket(t.bucket).Objects(ctx, &storage.Query{Prefix: watchConfigKeyPrefix}),
	}
}

type WatchConfigIDIterator struct {
	table *WatchConfigTable
	inner *storage.ObjectIterator
}

func (it *WatchConfigIDIterator) Next(ctx context.Context) (uint64, error) {
	tracer := otel.Tracer("row-major/rumor-mill/table")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "WatchConfigIDIterator.Next")
	defer span.End()

	for {
		attr, err := it.inner.Next()
		if err != nil {
			return 0, err
		}

		id, err := it.table.idFromGCSName(attr.Name)
		if err != nil {
			return 0, fmt.Errorf("while parsing ID: %w", err)
		}

		return id, nil
	}
}

func (t *WatchConfigTable) ListIDs(ctx context.Context) *WatchConfigIDIterator {
	return &WatchConfigIDIterator{
		table: t,
		inner: t.gcs.Bucket(t.bucket).Objects(ctx, &storage.Query{Prefix: watchConfigKeyPrefix}),
	}
}
