package webui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"row-major/artboard/dblayer"
	"row-major/artboard/dbtypes"
	"row-major/artboard/gcsurl"
	"row-major/artboard/webui/uitemplates"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
)

func viewObjectLink(collectionID, name string) string {
	return viewObjectLinkWithTagError(collectionID, name, "", "")
}

// viewObjectLinkWithTagError links to the object viewer, scrolled to the tags
// section, showing userError and pre-filling the add-tag form with tag.
func viewObjectLinkWithTagError(collectionID, name, userError, tag string) string {
	q := url.Values{}
	q.Add("collection", collectionID)
	q.Add("name", name)
	if userError != "" {
		q.Add("user-error", userError)
	}
	if tag != "" {
		q.Add("tag", tag)
	}
	link := &url.URL{
		Path:     "/view-object",
		RawQuery: q.Encode(),
	}
	if userError != "" {
		link.Fragment = "tags"
	}
	return link.String()
}

func addObjectTagLink() string {
	return "/add-object-tag"
}

// viewObjectLinkWithCaptionError links to the object viewer, scrolled to the
// caption section, showing userError and pre-filling the caption form with
// caption (which may be empty).
func viewObjectLinkWithCaptionError(collectionID, name, userError, caption string) string {
	q := url.Values{}
	q.Add("collection", collectionID)
	q.Add("name", name)
	if userError != "" {
		q.Add("caption-error", userError)
	}
	q.Add("caption", caption)
	link := &url.URL{
		Path:     "/view-object",
		RawQuery: q.Encode(),
		Fragment: "caption",
	}
	return link.String()
}

func setObjectCaptionLink() string {
	return "/set-object-caption"
}

// maxCaptionLength is the maximum length of a caption, in characters.
const maxCaptionLength = 50000

// normalizeCaption cleans up a user-entered caption, returning a user-facing
// error if it is not acceptable.  An empty caption is allowed, and clears the
// object's caption.  Line breaks (which browsers submit from a textarea as
// "\r\n") are normalized to "\n"; tabs are allowed, but other control
// characters are not.
func normalizeCaption(caption string) (string, error) {
	caption = strings.ReplaceAll(caption, "\r\n", "\n")
	caption = strings.TrimSpace(caption)
	if !utf8.ValidString(caption) {
		return "", errors.New("Caption must be valid UTF-8")
	}
	if strings.ContainsFunc(caption, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t'
	}) {
		return "", errors.New("Caption must not contain control characters")
	}
	if utf8.RuneCountInString(caption) > maxCaptionLength {
		return "", fmt.Errorf("Caption must be at most %d characters", maxCaptionLength)
	}
	return caption, nil
}

// maxTagLength is the maximum length of a tag, in characters.
const maxTagLength = 100

// normalizeTag cleans up a user-entered tag, returning a user-facing error if
// it is not acceptable.
func normalizeTag(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", errors.New("Tag must not be empty")
	}
	if !utf8.ValidString(tag) {
		return "", errors.New("Tag must be valid UTF-8")
	}
	if strings.ContainsFunc(tag, unicode.IsControl) {
		return "", errors.New("Tag must not contain control characters")
	}
	if utf8.RuneCountInString(tag) > maxTagLength {
		return "", fmt.Errorf("Tag must be at most %d characters", maxTagLength)
	}
	return tag, nil
}

func objectContentLink(collectionID, name string) string {
	q := url.Values{}
	q.Add("collection", collectionID)
	q.Add("name", name)
	link := &url.URL{
		Path:     "/collection-object",
		RawQuery: q.Encode(),
	}
	return link.String()
}

// validObjectName reports whether name (relative to the collection prefix)
// names a servable object, as opposed to nothing or a folder placeholder.
func validObjectName(name string) bool {
	return name != "" && !strings.HasSuffix(name, "/")
}

// loadCollection looks up a collection the user may access, along with its
// parsed storage location.  On failure, it writes an error response and returns
// nil.
func (u *WebUI) loadCollection(w http.ResponseWriter, r *http.Request, user *dbtypes.User, collectionID string) (*dbtypes.Collection, *gcsurl.URL) {
	ctx := r.Context()

	collection, err := u.db.GetCollectionForUser(ctx, user, collectionID)
	if errors.Is(err, dblayer.ErrPermissionDenied) {
		slog.InfoContext(ctx, "User is not allowed to view collection", slog.String("collection", collectionID))
		http.Error(w, "Not Found", http.StatusNotFound)
		return nil, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting collection", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return nil, nil
	}

	loc, err := gcsurl.Parse(collection.StorageURL)
	if err != nil {
		slog.ErrorContext(ctx, "Stored collection has invalid storage URL", slog.String("collection", collection.ID), slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return nil, nil
	}

	return collection, loc
}

// viewObjectHandler renders a page showing a single object from a collection,
// with links to its neighbors.
func (u *WebUI) viewObjectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	collectionID := r.Form.Get("collection")
	name := r.Form.Get("name")

	user := u.checkSession(ctx, w, r, viewObjectLink(collectionID, name))
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}

	collection, loc := u.loadCollection(w, r, user, collectionID)
	if collection == nil {
		return
	}

	if !validObjectName(name) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	bucket := u.storageClient.Bucket(loc.Bucket)
	fullName := loc.Prefix + name

	var attrs *storage.ObjectAttrs
	var prevName, nextName string
	var metadata *dbtypes.ObjectMetadata
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		metadata, err = u.db.GetObjectMetadata(gctx, collection.ID, name)
		return err
	})
	g.Go(func() error {
		var err error
		attrs, err = bucket.Object(fullName).Attrs(gctx)
		return err
	})
	g.Go(func() error {
		var err error
		prevName, err = previousObjectName(gctx, bucket, loc.Prefix, fullName)
		return err
	})
	g.Go(func() error {
		var err error
		nextName, err = nextObjectName(gctx, bucket, loc.Prefix, fullName)
		return err
	})
	err := g.Wait()
	if errors.Is(err, storage.ErrObjectNotExist) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "Error while looking up object", slog.String("collection", collection.ID), slog.String("object", fullName), slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	params := &uitemplates.ViewObjectParams{
		CollectionDisplayName: collection.DisplayName,
		CollectionLink:        showCollectionLink(collection.ID, ""),
		Name:                  name,
		SelfLink:              viewObjectLink(collection.ID, name),
		ContentLink:           objectContentLink(collection.ID, name),
		ContentType:           attrs.ContentType,
		Size:                  humanBytes(attrs.Size),
		Updated:               attrs.Updated.UTC().Format(time.DateTime),
		IsImage:               strings.HasPrefix(attrs.ContentType, "image/"),
		IsVideo:               strings.HasPrefix(attrs.ContentType, "video/"),
		Tags:                  metadata.Tags,
		AddTagLink:            addObjectTagLink(),
		CollectionID:          collection.ID,
		UserError:             r.Form.Get("user-error"),
		TagInput:              r.Form.Get("tag"),
		SetCaptionLink:        setObjectCaptionLink(),
		CaptionError:          r.Form.Get("caption-error"),
		CaptionInput:          metadata.Caption,
	}
	// After a failed (or interrupted) caption update, show what the user
	// typed rather than the stored caption, so they can fix it up.
	if r.Form.Has("caption") {
		params.CaptionInput = r.Form.Get("caption")
	}
	if prevName != "" {
		params.PrevLink = viewObjectLink(collection.ID, strings.TrimPrefix(prevName, loc.Prefix))
	}
	if nextName != "" {
		params.NextLink = viewObjectLink(collection.ID, strings.TrimPrefix(nextName, loc.Prefix))
	}

	content, err := uitemplates.ViewObjectPage(params)
	if err != nil {
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
		return
	}
}

// addObjectTagHandler applies a tag to an object in a collection, then
// redirects back to the object viewer.
func (u *WebUI) addObjectTagHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	collectionID := r.PostForm.Get("collection")
	name := r.PostForm.Get("name")
	rawTag := r.PostForm.Get("tag")

	// If the user needs to log in, the tag is lost, but they land back on the
	// object with the tag pre-filled.
	user := u.checkSession(ctx, w, r, viewObjectLinkWithTagError(collectionID, name, "", rawTag))
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}

	collection, loc := u.loadCollection(w, r, user, collectionID)
	if collection == nil {
		// loadCollection already wrote an error.
		return
	}

	if !validObjectName(name) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	tag, err := normalizeTag(rawTag)
	if err != nil {
		http.Redirect(w, r, viewObjectLinkWithTagError(collection.ID, name, err.Error(), rawTag), http.StatusSeeOther)
		return
	}

	// Only record metadata for objects that actually exist.
	fullName := loc.Prefix + name
	_, err = u.storageClient.Bucket(loc.Bucket).Object(fullName).Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "Error while looking up object", slog.String("collection", collection.ID), slog.String("object", fullName), slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if err := u.db.AddObjectTag(ctx, collection.ID, name, tag); err != nil {
		slog.ErrorContext(ctx, "Error while adding object tag", slog.String("collection", collection.ID), slog.String("object", fullName), slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	link := viewObjectLink(collection.ID, name) + "#tags"
	http.Redirect(w, r, link, http.StatusSeeOther)
}

// setObjectCaptionHandler sets the caption of an object in a collection, then
// redirects back to the object viewer.
func (u *WebUI) setObjectCaptionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	collectionID := r.PostForm.Get("collection")
	name := r.PostForm.Get("name")
	rawCaption := r.PostForm.Get("caption")

	// If the user needs to log in, the update is lost, but they land back on
	// the object with the caption pre-filled.
	user := u.checkSession(ctx, w, r, viewObjectLinkWithCaptionError(collectionID, name, "", rawCaption))
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}

	collection, loc := u.loadCollection(w, r, user, collectionID)
	if collection == nil {
		// loadCollection already wrote an error.
		return
	}

	if !validObjectName(name) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	caption, err := normalizeCaption(rawCaption)
	if err != nil {
		http.Redirect(w, r, viewObjectLinkWithCaptionError(collection.ID, name, err.Error(), rawCaption), http.StatusSeeOther)
		return
	}

	// Only record metadata for objects that actually exist.
	fullName := loc.Prefix + name
	_, err = u.storageClient.Bucket(loc.Bucket).Object(fullName).Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "Error while looking up object", slog.String("collection", collection.ID), slog.String("object", fullName), slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if err := u.db.SetObjectCaption(ctx, collection.ID, name, caption); err != nil {
		slog.ErrorContext(ctx, "Error while setting object caption", slog.String("collection", collection.ID), slog.String("object", fullName), slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	link := viewObjectLink(collection.ID, name) + "#caption"
	http.Redirect(w, r, link, http.StatusSeeOther)
}

// nextObjectName returns the full name of the first object under prefix that
// sorts after fullName, skipping folder placeholders.  It returns "" if there
// is none.
func nextObjectName(ctx context.Context, bucket *storage.BucketHandle, prefix, fullName string) (string, error) {
	q := &storage.Query{Prefix: prefix, StartOffset: fullName}
	if err := q.SetAttrSelection([]string{"Name"}); err != nil {
		return "", err
	}
	it := bucket.Objects(ctx, q)
	for {
		obj, err := it.Next()
		if err == iterator.Done {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("while listing objects after %q: %w", fullName, err)
		}
		if obj.Name == fullName || !validObjectName(strings.TrimPrefix(obj.Name, prefix)) {
			continue
		}
		return obj.Name, nil
	}
}

// previousObjectName returns the full name of the last object under prefix that
// sorts before fullName, skipping folder placeholders.  It returns "" if there
// is none.
//
// Cloud Storage can only list in ascending order, so this lists everything
// before fullName.  That is linear in the object's position in the collection.
func previousObjectName(ctx context.Context, bucket *storage.BucketHandle, prefix, fullName string) (string, error) {
	q := &storage.Query{Prefix: prefix, EndOffset: fullName}
	if err := q.SetAttrSelection([]string{"Name"}); err != nil {
		return "", err
	}
	it := bucket.Objects(ctx, q)
	prev := ""
	for {
		obj, err := it.Next()
		if err == iterator.Done {
			return prev, nil
		}
		if err != nil {
			return "", fmt.Errorf("while listing objects before %q: %w", fullName, err)
		}
		if !validObjectName(strings.TrimPrefix(obj.Name, prefix)) {
			continue
		}
		prev = obj.Name
	}
}

// objectContentHandler serves the content of an object from a collection,
// supporting single-part HTTP range requests.
func (u *WebUI) objectContentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	collectionID := r.Form.Get("collection")
	name := r.Form.Get("name")

	// This endpoint is fetched by <img> and <video> elements, so a redirect to
	// the login page would not be useful.  Just fail.
	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	collection, loc := u.loadCollection(w, r, user, collectionID)
	if collection == nil {
		return
	}

	if !validObjectName(name) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	fullName := loc.Prefix + name

	// ReadCompressed makes GCS return the stored bytes verbatim (no
	// decompressive transcoding), so that byte ranges and sizes line up with
	// the object metadata.  We pass the Content-Encoding through instead.
	obj := u.storageClient.Bucket(loc.Bucket).Object(fullName).ReadCompressed(true)
	attrs, err := obj.Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting object attributes", slog.String("collection", collection.ID), slog.String("object", fullName), slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	// Pin the generation so that the content we read matches the attributes
	// (and ETag) we report, even if the object is concurrently overwritten.
	obj = obj.Generation(attrs.Generation)

	etag := `"` + strconv.FormatInt(attrs.Generation, 10) + `"`
	lastModified := attrs.Updated.UTC().Format(http.TimeFormat)

	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Last-Modified", lastModified)
	h.Set("Accept-Ranges", "bytes")
	h.Set("Cache-Control", "private, max-age=3600")
	h.Set("Vary", "Cookie")
	// The bucket contents are not under our control.  Make sure that an HTML
	// or SVG object can't run script in our origin.
	h.Set("Content-Security-Policy", "sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	if disposition := mime.FormatMediaType("inline", map[string]string{"filename": path.Base(name)}); disposition != "" {
		h.Set("Content-Disposition", disposition)
	}
	contentType := attrs.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h.Set("Content-Type", contentType)
	if attrs.ContentEncoding != "" {
		h.Set("Content-Encoding", attrs.ContentEncoding)
	}

	if inm := r.Header.Get("If-None-Match"); inm != "" && etagListMatches(inm, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	size := attrs.Size
	start, length := int64(0), size
	status := http.StatusOK

	rangeHeader := r.Header.Get("Range")
	if ifRange := r.Header.Get("If-Range"); ifRange != "" && ifRange != etag && ifRange != lastModified {
		// The client's copy is stale; send the whole thing.
		rangeHeader = ""
	}
	if rangeHeader != "" {
		rs, ok := parseByteRange(rangeHeader, size)
		switch {
		case !ok:
			// Unsupported or malformed (including multi-range); ignore it.
		case rs.unsatisfiable:
			h.Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			h.Del("Content-Encoding")
			http.Error(w, "Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
			return
		default:
			start, length = rs.start, rs.length
			status = http.StatusPartialContent
			h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+length-1, size))
		}
	}

	h.Set("Content-Length", strconv.FormatInt(length, 10))

	if r.Method == http.MethodHead || length == 0 {
		w.WriteHeader(status)
		return
	}

	reader, err := obj.NewRangeReader(ctx, start, length)
	if err != nil {
		slog.ErrorContext(ctx, "Error while opening object", slog.String("collection", collection.ID), slog.String("object", fullName), slog.Any("err", err))
		h.Del("Content-Range")
		h.Del("Content-Encoding")
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.WriteHeader(status)
	if _, err := io.Copy(w, reader); err != nil {
		// It's too late to write an error to the HTTP response.  This also
		// happens routinely when a media element abandons a request.
		slog.InfoContext(ctx, "Error while copying object content", slog.String("object", fullName), slog.Any("err", err))
		return
	}
}

// byteRange is the result of resolving a Range header against a content size.
type byteRange struct {
	start, length int64
	unsatisfiable bool
}

// parseByteRange resolves a Range header against content of the given size.
// ok is false if the header is malformed or requests something we don't
// support (non-byte units, multiple ranges), in which case the caller should
// ignore it and serve the full content, as permitted by RFC 9110 section 14.2.
func parseByteRange(header string, size int64) (br byteRange, ok bool) {
	spec, found := strings.CutPrefix(header, "bytes=")
	if !found {
		return byteRange{}, false
	}
	spec = strings.TrimSpace(spec)
	if strings.Contains(spec, ",") {
		return byteRange{}, false
	}
	first, last, found := strings.Cut(spec, "-")
	if !found {
		return byteRange{}, false
	}
	first, last = strings.TrimSpace(first), strings.TrimSpace(last)

	if size == 0 {
		// Nothing to slice; just serve the (empty) whole.
		return byteRange{}, false
	}

	if first == "" {
		// Suffix range: the last N bytes.
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n < 0 {
			return byteRange{}, false
		}
		if n == 0 {
			return byteRange{unsatisfiable: true}, true
		}
		n = min(n, size)
		return byteRange{start: size - n, length: n}, true
	}

	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil || start < 0 {
		return byteRange{}, false
	}
	end := size - 1
	if last != "" {
		end, err = strconv.ParseInt(last, 10, 64)
		if err != nil || end < start {
			return byteRange{}, false
		}
	}
	if start >= size {
		return byteRange{unsatisfiable: true}, true
	}
	end = min(end, size-1)
	return byteRange{start: start, length: end - start + 1}, true
}

// etagListMatches reports whether an If-None-Match header value matches etag,
// using weak comparison.
func etagListMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		if strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}
