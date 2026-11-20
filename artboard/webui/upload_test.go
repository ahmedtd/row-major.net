package webui

import (
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
	if got, want := uploadObjectLink("abc", "", "", ""), "/upload-object?collection=abc"; got != want {
		t.Errorf("uploadObjectLink = %q, want %q", got, want)
	}
	if got, want := uploadObjectLink("abc", "a/b", "a/b/00000001.png", "oops"), "/upload-object?collection=abc&folder=a%2Fb&uploaded=a%2Fb%2F00000001.png&user-error=oops"; got != want {
		t.Errorf("uploadObjectLink = %q, want %q", got, want)
	}
}
