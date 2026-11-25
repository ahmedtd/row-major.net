package webui

import (
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
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"row-major/artboard/dbtypes"
	"row-major/artboard/gcsurl"
	"row-major/artboard/webui/uitemplates"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

const (
	// maxUploadBytes bounds the size of an upload request body.
	maxUploadBytes = 256 << 20

	// maxFolderLength bounds the length (in bytes) of the folder prefix that
	// uploads are placed under.  Cloud Storage object names can be at most
	// 1024 bytes, and the collection's own prefix needs room too.
	maxFolderLength = 512

	// maxNameAttempts is how many object numbers an upload tries before
	// giving up, if it keeps finding objects that already have the name.
	maxNameAttempts = 5

	// maxArchiveExtractedBytes bounds the total decompressed size of a zip
	// upload's contents, as a defense against zip bombs.
	maxArchiveExtractedBytes = 4 << 30

	// maxArchiveEntries bounds the number of entries read from a zip upload.
	maxArchiveEntries = 10000
)

var (
	errNoFile           = errors.New("No file was uploaded")
	errEmptyFile        = errors.New("The uploaded file is empty")
	errNameAttemptsUsed = errors.New("Could not find an unused object name")
	errNoArchiveMedia   = errors.New("The archive contains no image or video files")
	errTooManyEntries   = fmt.Errorf("The archive has more than %d entries", maxArchiveEntries)
)

// uploadStatus describes the outcome of an upload, for display on the upload
// form.
type uploadStatus struct {
	// Uploaded is the name of the (last) object uploaded, if any.
	Uploaded string

	// Count is the number of objects uploaded from an archive, and Skipped
	// the number of archive entries that were not uploaded.  Both are zero
	// for a single-file upload.
	Count   int
	Skipped int

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
	if status.Count != 0 {
		q.Add("uploaded-count", strconv.Itoa(status.Count))
	}
	if status.Skipped != 0 {
		q.Add("skipped-count", strconv.Itoa(status.Skipped))
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
	if n, err := strconv.Atoi(r.Form.Get("uploaded-count")); err == nil && n > 0 {
		params.UploadedCount = n
	}
	if n, err := strconv.Atoi(r.Form.Get("skipped-count")); err == nil && n > 0 {
		params.SkippedCount = n
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

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
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
				// Keep what the user typed, so they can correct it.  Any
				// objects already stored from an archive are still
				// reported.
				http.Redirect(w, r, uploadObjectLink(collection.ID, rawFolder, status), http.StatusSeeOther)
				return
			}
			slog.InfoContext(ctx, "Stored upload", slog.String("collection", collection.ID), slog.String("object", status.Uploaded), slog.Int("archive-count", status.Count), slog.String("user", user.ID))
			// Show the normalized folder, so the user sees where the next
			// upload will go.
			http.Redirect(w, r, uploadObjectLink(collection.ID, folder, status), http.StatusSeeOther)
			return
		}
	}
}

// storeUpload stores an uploaded file in the collection.  A zip archive is
// extracted, and each image or video in it is stored as a separate object.
func (u *WebUI) storeUpload(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, folder string, part *multipart.Part) uploadStatus {
	logErr := func(err error) {
		slog.ErrorContext(ctx, "Error while storing upload", slog.String("collection", collection.ID), slog.String("folder", folder), slog.Any("err", err))
	}

	// The buffer is large because the zip reader reads through it too.
	content := bufio.NewReaderSize(part, 64<<10)
	head, err := content.Peek(512)
	if err != nil && err != io.EOF {
		logErr(err)
		return uploadStatus{UserError: uploadUserError(err)}
	}
	if len(head) == 0 {
		return uploadStatus{UserError: errEmptyFile.Error()}
	}

	if isZipUpload(part.Header.Get("Content-Type"), part.FileName(), head) {
		status, err := u.storeArchive(ctx, collection, loc, folder, content)
		if err != nil {
			logErr(err)
			status.UserError = uploadUserError(err)
			if status.Count > 0 {
				status.UserError += fmt.Sprintf("  (%d files from the archive were uploaded before the failure.)", status.Count)
			}
		}
		return status
	}

	contentType := uploadContentType(part.Header.Get("Content-Type"), part.FileName(), head)
	ext := uploadExtension(contentType, part.FileName())
	name, err := u.storeObject(ctx, collection, loc, folder, contentType, ext, content)
	if err != nil {
		logErr(err)
		return uploadStatus{UserError: uploadUserError(err)}
	}
	return uploadStatus{Uploaded: name}
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

// storeArchive extracts a zip archive, storing each image or video in it as a
// separate object, in archive order.  It returns the status so far even on
// error, since earlier objects remain stored.
func (u *WebUI) storeArchive(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, folder string, content io.Reader) (uploadStatus, error) {
	status := uploadStatus{}
	zs := newZipStream(content, maxArchiveExtractedBytes)
	for entries := 0; ; entries++ {
		entry, err := zs.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status, err
		}
		if entries >= maxArchiveEntries {
			return status, errTooManyEntries
		}

		if !entry.supported() {
			if !strings.HasSuffix(entry.Name, "/") {
				status.Skipped++
			}
			continue
		}

		entryContent := bufio.NewReaderSize(entry, 512)
		head, err := entryContent.Peek(512)
		if err != nil && err != io.EOF {
			return status, err
		}
		contentType := archiveEntryContentType(entry.Name, head)
		if contentType == "" || len(head) == 0 {
			if !strings.HasSuffix(entry.Name, "/") {
				status.Skipped++
			}
			continue
		}

		name, err := u.storeObject(ctx, collection, loc, folder, contentType, uploadExtension(contentType, entry.Name), entryContent)
		if err != nil {
			return status, fmt.Errorf("while storing archive entry %q: %w", entry.Name, err)
		}
		status.Uploaded = name
		status.Count++
	}
	if status.Count == 0 {
		return status, errNoArchiveMedia
	}
	return status, nil
}

// storeObject writes content into the collection under the next object
// number, and returns its name relative to the collection prefix.
func (u *WebUI) storeObject(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, folder, contentType, ext string, content io.Reader) (string, error) {
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

	// Cancelling the context is the only way to abandon a write without
	// creating the object.
	writeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// The precondition guards against something else creating the object
	// between the check above and now.
	wr := bucket.Object(loc.Prefix + name).If(storage.Conditions{DoesNotExist: true}).NewWriter(writeCtx)
	wr.ContentType = contentType
	if _, err := io.Copy(wr, content); err != nil {
		cancel()
		wr.Close()
		return "", fmt.Errorf("while writing object %q: %w", name, err)
	}
	if err := wr.Close(); err != nil {
		return "", fmt.Errorf("while writing object %q: %w", name, err)
	}

	// Record metadata now, so the object is searchable without waiting for a
	// resync.  The object is already stored, so don't fail the upload over it.
	if _, _, err := u.db.EnsureObjectMetadata(ctx, collection.ID, []string{name}); err != nil {
		slog.ErrorContext(ctx, "Error while creating metadata for uploaded object", slog.String("collection", collection.ID), slog.String("object", name), slog.Any("err", err))
	}

	return name, nil
}

// uploadUserError turns an upload failure into a message for the user.
func uploadUserError(err error) string {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return fmt.Sprintf("File is too large; the limit is %s", humanBytes(maxUploadBytes))
	}
	for _, userErr := range []error{errNoFile, errEmptyFile, errNameAttemptsUsed, errNoArchiveMedia, errTooManyEntries, errZipFormat, errZipTooLarge, errZipChecksum, errZipUnsupported} {
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
