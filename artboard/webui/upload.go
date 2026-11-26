package webui

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

	"row-major/artboard/dblayer"
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
		CollectionID:          collection.ID,
		FolderUploadLink:      uploadFolderFileLink(collection.ID),
		MaxFileBytes:          maxUploadBytes,
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

	// The server's read and write timeouts cover the whole request, which
	// isn't long enough to receive a large upload.  Now that the user is
	// known, give them as long as the body keeps arriving.
	deadlines := &deadlineExtendingReader{r: r.Body, rc: http.NewResponseController(w)}
	deadlines.extend()
	r.Body = http.MaxBytesReader(w, deadlines, maxRequestBytes)
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
			// Finishing the upload after the body arrived may have taken a
			// while; leave time to write the response.
			deadlines.extend()
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

// uploadIdleTimeout is how long an upload may go without receiving data (or,
// once the body is received, without the response being written).
const uploadIdleTimeout = 30 * time.Second

// deadlineExtendingReader reads a request body, pushing the connection's read
// and write deadlines uploadIdleTimeout into the future as data arrives.
type deadlineExtendingReader struct {
	r            io.ReadCloser
	rc           *http.ResponseController
	lastExtended time.Time
}

func (d *deadlineExtendingReader) Read(p []byte) (int, error) {
	// Extending on every read would be wasteful; once a second is plenty.
	if time.Since(d.lastExtended) > time.Second {
		d.extend()
	}
	return d.r.Read(p)
}

func (d *deadlineExtendingReader) Close() error {
	return d.r.Close()
}

// extend pushes the deadlines out.  The write deadline moves too, since it
// also counts from the start of the request, and the response is only
// written after the body is consumed.
func (d *deadlineExtendingReader) extend() {
	d.lastExtended = time.Now()
	deadline := d.lastExtended.Add(uploadIdleTimeout)
	if err := d.rc.SetReadDeadline(deadline); err != nil {
		slog.Error("Error while extending upload read deadline", slog.Any("err", err))
	}
	if err := d.rc.SetWriteDeadline(deadline); err != nil {
		slog.Error("Error while extending upload write deadline", slog.Any("err", err))
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
		// content, not part: the peek above already consumed the start of
		// the part into content's buffer.
		archive, err := u.spoolArchive(ctx, content)
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

// copyStats records where the time went in a timedCopy.
type copyStats struct {
	bytes     int64
	readTime  time.Duration
	writeTime time.Duration
	total     time.Duration
}

func (c copyStats) String() string {
	return fmt.Sprintf("copied %d bytes in %v: %v reading, %v writing", c.bytes, c.total.Round(time.Millisecond), c.readTime.Round(time.Millisecond), c.writeTime.Round(time.Millisecond))
}

func (c copyStats) logAttrs(err error) []any {
	attrs := []any{
		slog.Int64("bytes", c.bytes),
		slog.Duration("total", c.total),
		slog.Duration("read", c.readTime),
		slog.Duration("write", c.writeTime),
	}
	if c.readTime > 0 {
		attrs = append(attrs, slog.Float64("read-mib-per-sec", float64(c.bytes)/(1<<20)/c.readTime.Seconds()))
	}
	if c.writeTime > 0 {
		attrs = append(attrs, slog.Float64("write-mib-per-sec", float64(c.bytes)/(1<<20)/c.writeTime.Seconds()))
	}
	if err != nil {
		attrs = append(attrs, slog.Any("err", err))
	}
	return attrs
}

// timedCopy is io.Copy, but measures time spent reading from src separately
// from time spent writing to dst.
func timedCopy(dst io.Writer, src io.Reader) (copyStats, error) {
	var stats copyStats
	start := time.Now()

	buf := make([]byte, 1<<20)
	for {
		t := time.Now()
		n, readErr := src.Read(buf)
		stats.readTime += time.Since(t)

		if n > 0 {
			t = time.Now()
			_, err := dst.Write(buf[:n])
			stats.writeTime += time.Since(t)
			if err != nil {
				stats.total = time.Since(start)
				return stats, err
			}
			stats.bytes += int64(n)
		}
		if readErr == io.EOF {
			stats.total = time.Since(start)
			return stats, nil
		}
		if readErr != nil {
			stats.total = time.Since(start)
			return stats, readErr
		}
	}
}

// spoolArchive saves a zip archive to the scratch directory, and checks that
// it is valid and within limits.  The caller must close the result.
func (u *WebUI) spoolArchive(ctx context.Context, content io.Reader) (_ *spooledArchive, retErr error) {
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

	stats, err := timedCopy(f, io.LimitReader(content, maxArchiveBytes+1))
	slog.InfoContext(ctx, "Spooled archive", stats.logAttrs(err)...)
	size := stats.bytes
	if err != nil {
		return nil, fmt.Errorf("while spooling archive (%s): %w", stats, err)
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
	return u.storeNamedEntry(ctx, collection, loc, name, zf.Name, rc)
}

// storeNamedEntry stores content as the named object, if it is an image or
// video (judged by sourcePath, its path in the archive or folder it came from).
// An object that already exists is left alone, so that a failed upload can be
// retried.
func (u *WebUI) storeNamedEntry(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, name, sourcePath string, r io.Reader) (entryOutcome, error) {
	content := bufio.NewReaderSize(r, 512)
	head, err := content.Peek(512)
	if err != nil && err != io.EOF {
		return entrySkipped, err
	}
	contentType := archiveEntryContentType(sourcePath, head)
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

// folderFileResponse is the JSON response to a folder file upload.
type folderFileResponse struct {
	// Outcome is "stored", "existed", or "skipped" (not an image or video).
	Outcome string `json:"outcome,omitempty"`

	// Name is the object's name within the collection.
	Name string `json:"name,omitempty"`

	// Error is a user-facing error message, if the upload failed.
	Error string `json:"error,omitempty"`
}

func writeFolderFileResponse(ctx context.Context, w http.ResponseWriter, code int, resp folderFileResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
	}
}

// uploadFolderFileLink is the endpoint that the folder uploader sends each
// file to.  The parameters other than the collection are added by the
// client.
func uploadFolderFileLink(collectionID string) string {
	q := url.Values{}
	q.Add("collection", collectionID)
	link := &url.URL{
		Path:     "/upload-folder-file",
		RawQuery: q.Encode(),
	}
	return link.String()
}

// uploadFolderFileHandler stores one file from a folder that the user picked
// in the browser.  The request body is the raw file content, and the query
// parameters give the collection, the destination folder, and the file's
// path within the picked folder.  The object is named after that path, under
// the destination folder.  Like archive entries, files that aren't images or
// videos are skipped, and existing objects are left alone, so the client can
// simply retry a folder.
//
// It responds with JSON (folderFileResponse), since it is called by script
// rather than by a form.
func (u *WebUI) uploadFolderFileHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	collectionID := q.Get("collection")
	sourcePath := q.Get("path")

	fail := func(code int, userError string) {
		writeFolderFileResponse(ctx, w, code, folderFileResponse{Error: userError})
	}

	// Unlike checkSession, don't redirect to log in: the client can't follow
	// that usefully.
	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		fail(http.StatusInternalServerError, "The upload failed.  Please try again.")
		return
	}
	if user == nil {
		fail(http.StatusUnauthorized, "You are not logged in.  Reload the page to log in, then retry.")
		return
	}

	collection, err := u.db.GetCollectionForUser(ctx, user, collectionID)
	if errors.Is(err, dblayer.ErrPermissionDenied) {
		fail(http.StatusNotFound, "Collection not found")
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting collection", slog.Any("err", err))
		fail(http.StatusInternalServerError, "The upload failed.  Please try again.")
		return
	}
	loc, err := gcsurl.Parse(collection.StorageURL)
	if err != nil {
		slog.ErrorContext(ctx, "Stored collection has invalid storage URL", slog.String("collection", collection.ID), slog.Any("err", err))
		fail(http.StatusInternalServerError, "The upload failed.  Please try again.")
		return
	}

	folder, err := normalizeFolder(q.Get("folder"))
	if err != nil {
		fail(http.StatusBadRequest, err.Error())
		return
	}
	name, ok := archiveObjectName(folder, sourcePath)
	if !ok || len(loc.Prefix)+len(name) > maxObjectNameLength {
		fail(http.StatusBadRequest, "The file's path can't be used as an object name")
		return
	}

	deadlines := &deadlineExtendingReader{r: r.Body, rc: http.NewResponseController(w)}
	deadlines.extend()
	body := http.MaxBytesReader(w, deadlines, maxUploadBytes)

	outcome, err := u.storeNamedEntry(ctx, collection, loc, name, sourcePath, body)
	deadlines.extend()
	if err != nil {
		slog.ErrorContext(ctx, "Error while storing folder file", slog.String("collection", collection.ID), slog.String("object", name), slog.Any("err", err))
		code := http.StatusInternalServerError
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			code = http.StatusRequestEntityTooLarge
		}
		fail(code, uploadUserError(err))
		return
	}

	resp := folderFileResponse{Name: name}
	switch outcome {
	case entryStored:
		resp.Outcome = "stored"
		slog.InfoContext(ctx, "Stored folder file", slog.String("collection", collection.ID), slog.String("object", name), slog.String("user", user.ID))
	case entryExisted:
		resp.Outcome = "existed"
	case entrySkipped:
		resp.Outcome = "skipped"
	}
	writeFolderFileResponse(ctx, w, http.StatusOK, resp)
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
