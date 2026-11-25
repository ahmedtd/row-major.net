package webui

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"row-major/artboard/dbtypes"
	"row-major/artboard/gcsurl"
	"row-major/artboard/webui/uitemplates"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

const (
	// maxUploadBytes bounds the size of a single uploaded file.
	maxUploadBytes = 256 << 20

	// maxArchiveBytes bounds the size of an uploaded zip archive, which is
	// spooled to the scratch directory.
	maxArchiveBytes = 2 << 30

	// maxRequestBytes bounds the size of an upload request body.  It leaves
	// room for the form fields around the file.
	maxRequestBytes = maxArchiveBytes + 1<<20

	// maxObjectNameLength is Cloud Storage's limit on object names, in bytes.
	maxObjectNameLength = 1024

	// maxArchiveProcessingTime bounds how long background extraction of an
	// archive may take.
	maxArchiveProcessingTime = 2 * time.Hour

	// scratchPattern names the files archives are spooled to.
	scratchPattern = "artboard-upload-*.zip"

	// maxFolderLength bounds the length (in bytes) of the folder prefix that
	// uploads are placed under.  Cloud Storage object names can be at most
	// 1024 bytes, and the collection's own prefix needs room too.
	maxFolderLength = 512

	// maxNameAttempts is how many object numbers an upload tries before
	// giving up, if it keeps finding objects that already have the name.
	maxNameAttempts = 5

	// maxArchiveExtractedBytes bounds the total decompressed size of a zip
	// upload's contents, as a defense against zip bombs.
	maxArchiveExtractedBytes = 8 << 30

	// maxArchiveEntries bounds the number of entries in a zip upload.
	maxArchiveEntries = 50000
)

var (
	errNoFile           = errors.New("No file was uploaded")
	errEmptyFile        = errors.New("The uploaded file is empty")
	errNameAttemptsUsed = errors.New("Could not find an unused object name")
	errTooManyEntries   = fmt.Errorf("The archive has more than %d entries", maxArchiveEntries)
	errTooLarge         = fmt.Errorf("The upload is too large; files can be at most %s, and zip archives %s", humanBytes(maxUploadBytes), humanBytes(maxArchiveBytes))

	errArchiveExpandsTooMuch = fmt.Errorf("The archive's contents are larger than %s", humanBytes(maxArchiveExtractedBytes))

	// errObjectExists means an object could not be written because one
	// already has its name.
	errObjectExists = errors.New("object already exists")
)

// uploadStatus describes the outcome of an upload, for display on the upload
// form.
type uploadStatus struct {
	// Uploaded is the name of the object uploaded, if any.
	Uploaded string

	// ArchiveFiles is the number of files in an uploaded zip archive, which
	// is being extracted in the background.
	ArchiveFiles int

	// UserError is an error from a failed upload.
	UserError string
}

// uploadObjectLink links to the upload form for a collection, with folder
// pre-filled, showing the outcome of the previous upload.
func uploadObjectLink(collectionID, folder string, status uploadStatus) string {
	q := url.Values{}
	q.Add("collection", collectionID)
	if folder != "" {
		q.Add("folder", folder)
	}
	if status.Uploaded != "" {
		q.Add("uploaded", status.Uploaded)
	}
	if status.ArchiveFiles != 0 {
		q.Add("archive-files", strconv.Itoa(status.ArchiveFiles))
	}
	if status.UserError != "" {
		q.Add("user-error", status.UserError)
	}
	link := &url.URL{
		Path:     "/upload-object",
		RawQuery: q.Encode(),
	}
	return link.String()
}

// uploadObjectPostLink is the target of the upload form.  The collection is
// passed in the URL (rather than as a form field) so that it is known before
// the request body is read.
func uploadObjectPostLink(collectionID string) string {
	q := url.Values{}
	q.Add("collection", collectionID)
	link := &url.URL{
		Path:     "/upload-object",
		RawQuery: q.Encode(),
	}
	return link.String()
}

// normalizeFolder cleans up a user-entered folder prefix, returning a
// user-facing error if it is not acceptable.  Leading, trailing, and repeated
// slashes are removed.  The result is either empty (the collection's top
// level) or a slash-separated path with no trailing slash.
func normalizeFolder(folder string) (string, error) {
	if !utf8.ValidString(folder) {
		return "", errors.New("Folder must be valid UTF-8")
	}
	if strings.ContainsFunc(folder, unicode.IsControl) {
		return "", errors.New("Folder must not contain control characters")
	}

	var segments []string
	for _, s := range strings.Split(strings.TrimSpace(folder), "/") {
		s = strings.TrimSpace(s)
		switch s {
		case "":
			continue
		case ".", "..":
			return "", errors.New(`Folder must not contain "." or ".." segments`)
		}
		segments = append(segments, s)
	}

	folder = strings.Join(segments, "/")
	if len(folder) > maxFolderLength {
		return "", fmt.Errorf("Folder must be at most %d bytes", maxFolderLength)
	}
	return folder, nil
}

// uploadObjectName returns the name (relative to the collection prefix) for
// the upload numbered n.  Numbers are zero-padded so that names sort in upload
// order.
func uploadObjectName(folder string, n int64, ext string) string {
	name := fmt.Sprintf("%08d%s", n, ext)
	if folder != "" {
		name = folder + "/" + name
	}
	return name
}

// uploadContentType determines the content type of an upload.  It prefers the
// type the browser declared, then a type implied by the file name's extension,
// and finally sniffs the content.  Browsers declare "application/octet-stream"
// when they don't know, so that is treated as no declaration.
func uploadContentType(declared, filename string, head []byte) string {
	if mt, _, err := mime.ParseMediaType(declared); err == nil && mt != "application/octet-stream" {
		return declared
	}
	if t := mime.TypeByExtension(path.Ext(filename)); t != "" {
		return t
	}
	return http.DetectContentType(head)
}

// preferredExtensions picks one extension for content types that have
// several, so that uploaded names look conventional.
var preferredExtensions = map[string]string{
	"image/avif":      ".avif",
	"image/bmp":       ".bmp",
	"image/gif":       ".gif",
	"image/heic":      ".heic",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/svg+xml":   ".svg",
	"image/tiff":      ".tiff",
	"image/webp":      ".webp",
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/webm":      ".webm",
	"application/pdf": ".pdf",
	"text/plain":      ".txt",
}

var extensionRE = regexp.MustCompile(`^\.[a-z0-9]{1,10}$`)

// uploadExtension picks the file extension (including the dot) for an
// uploaded object, or "" if there is no sensible one.
func uploadExtension(contentType, filename string) string {
	mt, _, _ := mime.ParseMediaType(contentType)
	if ext, ok := preferredExtensions[mt]; ok {
		return ext
	}
	if ext := strings.ToLower(path.Ext(filename)); extensionRE.MatchString(ext) {
		return ext
	}
	if exts, err := mime.ExtensionsByType(mt); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// uploadObjectGetHandler renders the upload form.
func (u *WebUI) uploadObjectGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	collectionID := r.Form.Get("collection")
	folder := r.Form.Get("folder")

	user := u.checkSession(ctx, w, r, uploadObjectLink(collectionID, folder, uploadStatus{}))
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}

	collection, _ := u.loadCollection(w, r, user, collectionID)
	if collection == nil {
		// loadCollection already wrote an error.
		return
	}

	params := &uitemplates.UploadObjectParams{
		CollectionDisplayName: collection.DisplayName,
		CollectionLink:        showCollectionLink(collection.ID, ""),
		SelfLink:              uploadObjectLink(collection.ID, "", uploadStatus{}),
		UploadLink:            uploadObjectPostLink(collection.ID),
		Folder:                folder,
		UserError:             r.Form.Get("user-error"),
	}
	if uploaded := r.Form.Get("uploaded"); validObjectName(uploaded) {
		params.UploadedName = uploaded
		params.UploadedViewLink = viewObjectLink(collection.ID, uploaded)
		params.UploadedContentLink = objectContentLink(collection.ID, uploaded)
		// Uploads are named with an extension derived from their content
		// type, so the extension is a reliable guide.
		params.UploadedIsImage = strings.HasPrefix(mime.TypeByExtension(path.Ext(uploaded)), "image/")
	}
	if n, err := strconv.Atoi(r.Form.Get("archive-files")); err == nil && n > 0 {
		params.ArchiveFiles = n
	}

	content, err := uitemplates.UploadObjectPage(params)
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

// uploadObjectPostHandler stores an uploaded file in a collection under the
// next object number, then redirects back to the upload form.
//
// The body is streamed straight through to Cloud Storage rather than buffered,
// so the form must send the folder field before the file.
func (u *WebUI) uploadObjectPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	collectionID := r.URL.Query().Get("collection")

	// If the user needs to log in, the upload is lost, but they land back on
	// the upload form.
	user := u.checkSession(ctx, w, r, uploadObjectLink(collectionID, "", uploadStatus{}))
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}

	collection, loc := u.loadCollection(w, r, user, collectionID)
	if collection == nil {
		// loadCollection already wrote an error.
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	rawFolder := ""
	folder := ""
	fail := func(userError string) {
		http.Redirect(w, r, uploadObjectLink(collection.ID, rawFolder, uploadStatus{UserError: userError}), http.StatusSeeOther)
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			fail(errNoFile.Error())
			return
		}
		if err != nil {
			slog.InfoContext(ctx, "Error while reading upload form", slog.String("collection", collection.ID), slog.Any("err", err))
			fail(uploadUserError(err))
			return
		}

		switch part.FormName() {
		case "folder":
			b, err := io.ReadAll(io.LimitReader(part, 4*maxFolderLength))
			if err != nil {
				fail(uploadUserError(err))
				return
			}
			rawFolder = string(b)
			folder, err = normalizeFolder(rawFolder)
			if err != nil {
				fail(err.Error())
				return
			}

		case "file":
			if part.FileName() == "" {
				// A file input with nothing selected still sends an empty
				// part.
				continue
			}
			status := u.storeUpload(ctx, collection, loc, folder, part)
			if status.UserError != "" {
				// Keep what the user typed, so they can correct it.
				http.Redirect(w, r, uploadObjectLink(collection.ID, rawFolder, status), http.StatusSeeOther)
				return
			}
			slog.InfoContext(ctx, "Stored upload", slog.String("collection", collection.ID), slog.String("object", status.Uploaded), slog.Int("archive-files", status.ArchiveFiles), slog.String("user", user.ID))
			// Show the normalized folder, so the user sees where the next
			// upload will go.
			http.Redirect(w, r, uploadObjectLink(collection.ID, folder, status), http.StatusSeeOther)
			return
		}
	}
}

// storeUpload stores an uploaded file in the collection.  A zip archive is
// spooled to the scratch directory, then extracted in the background, with
// each image or video in it stored as a separate object.
func (u *WebUI) storeUpload(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, folder string, part *multipart.Part) uploadStatus {
	logErr := func(err error) {
		slog.ErrorContext(ctx, "Error while storing upload", slog.String("collection", collection.ID), slog.String("folder", folder), slog.Any("err", err))
	}

	content := bufio.NewReaderSize(part, 512)
	head, err := content.Peek(512)
	if err != nil && err != io.EOF {
		logErr(err)
		return uploadStatus{UserError: uploadUserError(err)}
	}
	if len(head) == 0 {
		return uploadStatus{UserError: errEmptyFile.Error()}
	}

	if isZipUpload(part.Header.Get("Content-Type"), part.FileName(), head) {
		archive, err := u.spoolArchive(part)
		if err != nil {
			logErr(err)
			return uploadStatus{UserError: uploadUserError(err)}
		}
		files := archive.fileCount()

		// Extracting a large archive can take longer than the load
		// balancer will wait for a response, so it happens in the
		// background, detached from the request.
		//
		// TODO: Track these as jobs, so users can see progress and
		// failures, and so shutdown can wait for them.
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), maxArchiveProcessingTime)
		go func() {
			defer cancel()
			defer archive.close()
			u.extractArchive(bgCtx, collection, loc, folder, archive)
		}()
		return uploadStatus{ArchiveFiles: files}
	}

	contentType := uploadContentType(part.Header.Get("Content-Type"), part.FileName(), head)
	ext := uploadExtension(contentType, part.FileName())
	// The request body may be as large as an archive, so single files need
	// their own limit.
	capped := &cappedReader{r: content, remaining: maxUploadBytes}
	name, err := u.storeNumberedObject(ctx, collection, loc, folder, contentType, ext, capped)
	if err != nil {
		logErr(err)
		return uploadStatus{UserError: uploadUserError(err)}
	}
	return uploadStatus{Uploaded: name}
}

// cappedReader reads from r, failing with errTooLarge if it yields more than
// remaining bytes.
type cappedReader struct {
	r         io.Reader
	remaining int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.remaining -= int64(n)
	if c.remaining < 0 {
		return n, errTooLarge
	}
	return n, err
}

// zipContentTypes are the types browsers declare for zip archives.
var zipContentTypes = map[string]bool{
	"application/zip":              true,
	"application/x-zip":            true,
	"application/x-zip-compressed": true,
}

// isZipUpload reports whether an upload is a zip archive to be extracted.  It
// must both look like a zip and be labelled as one, since many other formats
// (such as .docx) are zips underneath.
func isZipUpload(declared, filename string, head []byte) bool {
	if !bytes.HasPrefix(head, []byte("PK\x03\x04")) && !bytes.HasPrefix(head, []byte("PK\x05\x06")) {
		return false
	}
	mt, _, _ := mime.ParseMediaType(declared)
	return zipContentTypes[mt] || strings.EqualFold(path.Ext(filename), ".zip")
}

// archiveMediaTypes maps extensions to content types for archive entries.
// Unlike uploads, archive entries have no declared type, and the server's
// MIME table may lack common video types.
var archiveMediaTypes = map[string]string{
	".avif": "image/avif",
	".bmp":  "image/bmp",
	".gif":  "image/gif",
	".heic": "image/heic",
	".heif": "image/heif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".webp": "image/webp",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".mp4":  "video/mp4",
	".webm": "video/webm",
}

// archiveEntryContentType determines the content type of an archive entry,
// returning "" if it should not be uploaded: it isn't an image or video, or
// it is a directory or hidden file.
func archiveEntryContentType(name string, head []byte) string {
	if strings.HasSuffix(name, "/") {
		return ""
	}
	// Skip hidden files and the resource-fork files macOS adds to archives
	// (in __MACOSX/, and named ._original.jpg).
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") || seg == "__MACOSX" {
			return ""
		}
	}

	ext := strings.ToLower(path.Ext(name))
	contentType := archiveMediaTypes[ext]
	if contentType == "" {
		contentType = uploadContentType("", name, head)
	}
	mt, _, _ := mime.ParseMediaType(contentType)
	if !strings.HasPrefix(mt, "image/") && !strings.HasPrefix(mt, "video/") {
		return ""
	}
	return contentType
}

// archiveObjectName returns the object name (relative to the collection
// prefix) for an archive entry: its path within the archive, under folder.
// It returns false if the entry's path is not usable as an object name.
func archiveObjectName(folder, entryName string) (string, bool) {
	if !utf8.ValidString(entryName) || strings.ContainsFunc(entryName, unicode.IsControl) {
		return "", false
	}
	// Some Windows archivers use backslashes despite the spec.
	entryName = strings.ReplaceAll(entryName, `\`, "/")

	var segments []string
	if folder != "" {
		segments = append(segments, folder)
	}
	for _, seg := range strings.Split(entryName, "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			return "", false
		}
		segments = append(segments, seg)
	}
	if len(segments) == 0 || (folder != "" && len(segments) == 1) {
		return "", false
	}
	return strings.Join(segments, "/"), true
}

// spooledArchive is an uploaded zip archive saved in the scratch directory.
type spooledArchive struct {
	f  *os.File
	zr *zip.Reader
}

// close deletes the archive's scratch file.
func (a *spooledArchive) close() {
	a.f.Close()
	if err := os.Remove(a.f.Name()); err != nil {
		slog.Error("Error while removing scratch file", slog.String("file", a.f.Name()), slog.Any("err", err))
	}
}

// fileCount returns the number of (non-directory) entries in the archive.
func (a *spooledArchive) fileCount() int {
	n := 0
	for _, zf := range a.zr.File {
		if !zf.FileInfo().IsDir() {
			n++
		}
	}
	return n
}

// spoolArchive saves a zip archive to the scratch directory, and checks that
// it is valid and within limits.  The caller must close the result.
func (u *WebUI) spoolArchive(content io.Reader) (_ *spooledArchive, retErr error) {
	f, err := os.CreateTemp(u.scratchDir, scratchPattern)
	if err != nil {
		return nil, fmt.Errorf("while creating scratch file: %w", err)
	}
	a := &spooledArchive{f: f}
	defer func() {
		if retErr != nil {
			a.close()
		}
	}()

	size, err := io.Copy(f, io.LimitReader(content, maxArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("while spooling archive (total bytes copied %d): %w", size, err)
	}
	if size > maxArchiveBytes {
		return nil, errTooLarge
	}

	a.zr, err = zip.NewReader(f, size)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		// Names are checked individually during extraction.
		return nil, fmt.Errorf("while opening archive: %w", err)
	}

	// archive/zip makes sure entries don't decompress to more than their
	// declared sizes, so the declared sizes can be trusted.
	if len(a.zr.File) > maxArchiveEntries {
		return nil, errTooManyEntries
	}
	var total uint64
	for _, zf := range a.zr.File {
		total += zf.UncompressedSize64
	}
	if total > maxArchiveExtractedBytes {
		return nil, errArchiveExpandsTooMuch
	}
	return a, nil
}

// extractArchive stores each image or video in an archive as an object named
// after its path in the archive.  Objects that already exist are left alone,
// so a failed upload can be retried.  It runs in the background, so the
// outcome is only logged.
func (u *WebUI) extractArchive(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, folder string, a *spooledArchive) {
	var stored, existing, skipped int
	logAttrs := func() []any {
		return []any{
			slog.String("collection", collection.ID),
			slog.String("folder", folder),
			slog.Int("stored", stored),
			slog.Int("existing", existing),
			slog.Int("skipped", skipped),
		}
	}

	for _, zf := range a.zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		name, ok := archiveObjectName(folder, zf.Name)
		if !ok || len(loc.Prefix)+len(name) > maxObjectNameLength || zf.Flags&0x1 != 0 {
			// Unusable name, or encrypted.
			skipped++
			continue
		}

		outcome, err := u.storeArchiveEntry(ctx, collection, loc, name, zf)
		if err != nil {
			slog.ErrorContext(ctx, "Error while extracting uploaded archive", append(logAttrs(), slog.String("entry", zf.Name), slog.Any("err", err))...)
			return
		}
		switch outcome {
		case entrySkipped:
			skipped++
		case entryExisted:
			existing++
		case entryStored:
			stored++
		}
	}
	slog.InfoContext(ctx, "Extracted uploaded archive", logAttrs()...)
}

type entryOutcome int

const (
	entrySkipped entryOutcome = iota
	entryExisted
	entryStored
)

// storeArchiveEntry stores one archive entry as the named object, if it is an
// image or video.
func (u *WebUI) storeArchiveEntry(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, name string, zf *zip.File) (entryOutcome, error) {
	rc, err := zf.Open()
	if errors.Is(err, zip.ErrAlgorithm) {
		// An unsupported compression method.
		return entrySkipped, nil
	}
	if err != nil {
		return entrySkipped, err
	}
	defer rc.Close()

	content := bufio.NewReaderSize(rc, 512)
	head, err := content.Peek(512)
	if err != nil && err != io.EOF {
		return entrySkipped, err
	}
	contentType := archiveEntryContentType(zf.Name, head)
	if contentType == "" || len(head) == 0 {
		return entrySkipped, nil
	}

	// Check first, to avoid uploading the content only to have the write's
	// precondition fail.
	_, err = u.storageClient.Bucket(loc.Bucket).Object(loc.Prefix + name).Attrs(ctx)
	if err == nil {
		u.ensureMetadata(ctx, collection, name)
		return entryExisted, nil
	}
	if !errors.Is(err, storage.ErrObjectNotExist) {
		return entrySkipped, fmt.Errorf("while checking for existing object %q: %w", name, err)
	}

	err = u.writeObject(ctx, collection, loc, name, contentType, content)
	if errors.Is(err, errObjectExists) {
		u.ensureMetadata(ctx, collection, name)
		return entryExisted, nil
	}
	if err != nil {
		return entrySkipped, err
	}
	return entryStored, nil
}

// storeNumberedObject writes content into the collection under the next
// object number, and returns its name relative to the collection prefix.
func (u *WebUI) storeNumberedObject(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, folder, contentType, ext string, content io.Reader) (string, error) {
	bucket := u.storageClient.Bucket(loc.Bucket)

	// The counter normally yields an unused name, but objects can also be
	// added to the bucket by other means.  Skip over any that are taken, so
	// that we fail before consuming the (unrepeatable) request body.
	name := ""
	for range maxNameAttempts {
		n, err := u.db.AllocateObjectNumber(ctx, collection.ID)
		if err != nil {
			return "", err
		}
		candidate := uploadObjectName(folder, n, ext)
		_, err = bucket.Object(loc.Prefix + candidate).Attrs(ctx)
		if errors.Is(err, storage.ErrObjectNotExist) {
			name = candidate
			break
		}
		if err != nil {
			return "", fmt.Errorf("while checking for existing object %q: %w", candidate, err)
		}
	}
	if name == "" {
		return "", errNameAttemptsUsed
	}

	if err := u.writeObject(ctx, collection, loc, name, contentType, content); err != nil {
		return "", err
	}
	return name, nil
}

// writeObject creates the named object (relative to the collection prefix)
// with the given content, and records its metadata.  It fails with
// errObjectExists if the object already exists.
func (u *WebUI) writeObject(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, name, contentType string, content io.Reader) error {
	// Cancelling the context is the only way to abandon a write without
	// creating the object.
	writeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// The precondition guards against something else creating the object
	// since the caller checked.
	wr := u.storageClient.Bucket(loc.Bucket).Object(loc.Prefix + name).If(storage.Conditions{DoesNotExist: true}).NewWriter(writeCtx)
	wr.ContentType = contentType
	if _, err := io.Copy(wr, content); err != nil {
		cancel()
		wr.Close()
		return fmt.Errorf("while writing object %q: %w", name, err)
	}
	if err := wr.Close(); err != nil {
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusPreconditionFailed {
			return errObjectExists
		}
		return fmt.Errorf("while writing object %q: %w", name, err)
	}

	u.ensureMetadata(ctx, collection, name)
	return nil
}

// ensureMetadata records metadata for a stored object, so it is searchable
// without waiting for a resync.  The object is already stored, so failure is
// only logged.
func (u *WebUI) ensureMetadata(ctx context.Context, collection *dbtypes.Collection, name string) {
	if _, _, err := u.db.EnsureObjectMetadata(ctx, collection.ID, []string{name}); err != nil {
		slog.ErrorContext(ctx, "Error while creating metadata for uploaded object", slog.String("collection", collection.ID), slog.String("object", name), slog.Any("err", err))
	}
}

// uploadUserError turns an upload failure into a message for the user.
func uploadUserError(err error) string {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return errTooLarge.Error()
	}
	if errors.Is(err, zip.ErrFormat) || errors.Is(err, zip.ErrChecksum) {
		return "The archive is not a valid zip file, or is corrupt"
	}
	for _, userErr := range []error{errNoFile, errEmptyFile, errTooLarge, errNameAttemptsUsed, errTooManyEntries, errArchiveExpandsTooMuch} {
		if errors.Is(err, userErr) {
			return userErr.Error()
		}
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && (apiErr.Code == http.StatusForbidden || apiErr.Code == http.StatusUnauthorized) {
		return "This site does not have write access to the collection's bucket.  Grant its service identity permission to create objects."
	}
	return "The upload failed.  Please try again."
}
