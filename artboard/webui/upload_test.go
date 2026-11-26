package webui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeFolder(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"  /  ", "", false},
		{"cats", "cats", false},
		{"/cats/tabby/", "cats/tabby", false},
		{"cats//tabby", "cats/tabby", false},
		{" cats / tabby ", "cats/tabby", false},
		{"cats/../dogs", "", true},
		{"./cats", "", true},
		{"cats\ttabby", "", true},
		{"\xff", "", true},
		{strings.Repeat("a", maxFolderLength), strings.Repeat("a", maxFolderLength), false},
		{strings.Repeat("a", maxFolderLength+1), "", true},
	} {
		got, err := normalizeFolder(tc.in)
		if got != tc.want || (err != nil) != tc.wantErr {
			t.Errorf("normalizeFolder(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestUploadObjectName(t *testing.T) {
	if got, want := uploadObjectName("", 7, ".png"), "00000007.png"; got != want {
		t.Errorf("uploadObjectName = %q, want %q", got, want)
	}
	if got, want := uploadObjectName("cats/tabby", 123456789, ""), "cats/tabby/123456789"; got != want {
		t.Errorf("uploadObjectName = %q, want %q", got, want)
	}
}

func TestUploadContentType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	for _, tc := range []struct {
		declared, filename string
		head               []byte
		want               string
	}{
		{"image/webp", "image.png", png, "image/webp"},
		{"", "photo.jpg", png, "image/jpeg"},
		{"application/octet-stream", "photo.jpg", png, "image/jpeg"},
		{"application/octet-stream", "", png, "image/png"},
		{"", "blob", png, "image/png"},
		{"not a type", "", []byte("hello"), "text/plain; charset=utf-8"},
	} {
		if got := uploadContentType(tc.declared, tc.filename, tc.head); got != tc.want {
			t.Errorf("uploadContentType(%q, %q) = %q, want %q", tc.declared, tc.filename, got, tc.want)
		}
	}
}

func TestUploadExtension(t *testing.T) {
	for _, tc := range []struct {
		contentType, filename, want string
	}{
		{"image/jpeg", "photo.jpeg", ".jpg"},
		{"image/png", "", ".png"},
		{"text/plain; charset=utf-8", "", ".txt"},
		{"application/x-made-up", "notes.MD", ".md"},
		{"application/x-made-up", "weird.ext with space", ""},
		{"application/x-made-up", "", ""},
		{"application/json", "", ".json"},
	} {
		if got := uploadExtension(tc.contentType, tc.filename); got != tc.want {
			t.Errorf("uploadExtension(%q, %q) = %q, want %q", tc.contentType, tc.filename, got, tc.want)
		}
	}
}

func TestUploadObjectLink(t *testing.T) {
	if got, want := uploadObjectLink("abc", "", uploadStatus{}), "/upload-object?collection=abc"; got != want {
		t.Errorf("uploadObjectLink = %q, want %q", got, want)
	}
	if got, want := uploadObjectLink("abc", "a/b", uploadStatus{Uploaded: "a/b/00000001.png", ArchiveFiles: 3, UserError: "oops"}), "/upload-object?archive-files=3&collection=abc&folder=a%2Fb&uploaded=a%2Fb%2F00000001.png&user-error=oops"; got != want {
		t.Errorf("uploadObjectLink = %q, want %q", got, want)
	}
}

func TestArchiveObjectName(t *testing.T) {
	for _, tc := range []struct {
		folder, entry string
		want          string
		wantOK        bool
	}{
		{"", "cat.jpg", "cat.jpg", true},
		{"", "photos/2024/cat.jpg", "photos/2024/cat.jpg", true},
		{"uploads", "photos/cat.jpg", "uploads/photos/cat.jpg", true},
		{"uploads", "/photos//./cat.jpg", "uploads/photos/cat.jpg", true},
		{"", `photos\cat.jpg`, "photos/cat.jpg", true},
		{"", "photos/../../cat.jpg", "", false},
		{"", "bad\tname.jpg", "", false},
		{"", "\xffname.jpg", "", false},
		{"uploads", "/", "", false},
	} {
		got, ok := archiveObjectName(tc.folder, tc.entry)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("archiveObjectName(%q, %q) = %q, %v; want %q, %v", tc.folder, tc.entry, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestCappedReader(t *testing.T) {
	if _, err := io.ReadAll(&cappedReader{r: strings.NewReader("12345"), remaining: 5}); err != nil {
		t.Errorf("reading exactly the cap: %v", err)
	}
	if _, err := io.ReadAll(&cappedReader{r: strings.NewReader("123456"), remaining: 5}); !errors.Is(err, errTooLarge) {
		t.Errorf("reading past the cap: err = %v, want errTooLarge", err)
	}
}

func TestIsZipUpload(t *testing.T) {
	zipHead := []byte("PK\x03\x04\x14\x00")
	for _, tc := range []struct {
		declared, filename string
		head               []byte
		want               bool
	}{
		{"application/zip", "photos.zip", zipHead, true},
		{"application/x-zip-compressed", "photos", zipHead, true},
		{"application/octet-stream", "PHOTOS.ZIP", zipHead, true},
		{"", "empty.zip", []byte("PK\x05\x06"), true},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "doc.docx", zipHead, false},
		{"application/zip", "fake.zip", []byte("not a zip"), false},
	} {
		if got := isZipUpload(tc.declared, tc.filename, tc.head); got != tc.want {
			t.Errorf("isZipUpload(%q, %q) = %v, want %v", tc.declared, tc.filename, got, tc.want)
		}
	}
}

func TestArchiveEntryContentType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	for _, tc := range []struct {
		name string
		head []byte
		want string
	}{
		{"photos/cat.JPG", png, "image/jpeg"},
		{"clip.mov", []byte("\x00\x00\x00\x14ftypqt  "), "video/quicktime"},
		{"noext", png, "image/png"},
		{"notes.txt", []byte("hello"), ""},
		{"photos/", nil, ""},
		{"__MACOSX/photos/._cat.jpg", png, ""},
		{"photos/._cat.jpg", png, ""},
		{".hidden/cat.png", png, ""},
	} {
		if got := archiveEntryContentType(tc.name, tc.head); got != tc.want {
			t.Errorf("archiveEntryContentType(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTimedCopy(t *testing.T) {
	src := strings.Repeat("artboard", 500000)
	var dst strings.Builder
	stats, err := timedCopy(&dst, strings.NewReader(src))
	if err != nil {
		t.Fatalf("timedCopy: %v", err)
	}
	if dst.String() != src || stats.bytes != int64(len(src)) {
		t.Errorf("copied %d bytes (stats say %d), want %d", dst.Len(), stats.bytes, len(src))
	}
}

func TestUploadFolderFileLink(t *testing.T) {
	if got, want := uploadFolderFileLink("abc"), "/upload-folder-file?collection=abc"; got != want {
		t.Errorf("uploadFolderFileLink = %q, want %q", got, want)
	}
}

func TestStaticHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	staticHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/static/uploadfolder.js?v=abc", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q, want text/javascript", ct)
	}
	if !strings.Contains(rec.Body.String(), "React.createElement") {
		t.Errorf("body does not look like uploadfolder.js")
	}
}
