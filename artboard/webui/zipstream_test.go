package webui

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"errors"
	"hash/crc32"
	"io"
	"math/rand/v2"
	"strings"
	"testing"
)

type testZipEntry struct {
	name string
	data []byte

	method uint16

	// raw writes the entry with its sizes in the local header, rather than
	// in a trailing data descriptor.
	raw bool

	// badCRC records a wrong checksum (only with raw).
	badCRC bool
}

func buildTestZip(t *testing.T, entries []testZipEntry) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for _, e := range entries {
		if !e.raw {
			w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: e.method})
			if err != nil {
				t.Fatalf("CreateHeader: %v", err)
			}
			if _, err := w.Write(e.data); err != nil {
				t.Fatalf("Write: %v", err)
			}
			continue
		}

		compressed := e.data
		if e.method == zip.Deflate {
			cb := &bytes.Buffer{}
			fw, _ := flate.NewWriter(cb, flate.DefaultCompression)
			fw.Write(e.data)
			fw.Close()
			compressed = cb.Bytes()
		}
		crc := crc32.ChecksumIEEE(e.data)
		if e.badCRC {
			crc++
		}
		w, err := zw.CreateRaw(&zip.FileHeader{
			Name:               e.name,
			Method:             e.method,
			CRC32:              crc,
			CompressedSize64:   uint64(len(compressed)),
			UncompressedSize64: uint64(len(e.data)),
		})
		if err != nil {
			t.Fatalf("CreateRaw: %v", err)
		}
		if _, err := w.Write(compressed); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.Bytes()
}

// readTestZip reads every entry in the archive, returning their contents by
// name.  Entries that can't be read map to nil.
func readTestZip(archive []byte, maxBytes int64) (map[string][]byte, []string, error) {
	got := map[string][]byte{}
	var order []string
	zs := newZipStream(bytes.NewReader(archive), maxBytes)
	for {
		e, err := zs.Next()
		if err == io.EOF {
			return got, order, nil
		}
		if err != nil {
			return got, order, err
		}
		order = append(order, e.Name)
		if !e.supported() {
			got[e.Name] = nil
			continue
		}
		data, err := io.ReadAll(e)
		if err != nil {
			return got, order, err
		}
		got[e.Name] = data
	}
}

func TestZipStream(t *testing.T) {
	big := bytes.Repeat([]byte("artboard "), 100000)
	// Stored data that contains a descriptor signature which isn't one.
	tricky := append([]byte("before PK\x07\x08"), bytes.Repeat([]byte{0}, 40)...)
	entries := []testZipEntry{
		{name: "dir/", method: zip.Store},
		{name: "dir/deflate-descriptor.txt", data: big, method: zip.Deflate},
		{name: "store-descriptor.bin", data: tricky, method: zip.Store},
		{name: "deflate-raw.txt", data: big, method: zip.Deflate, raw: true},
		{name: "store-raw.bin", data: tricky, method: zip.Store, raw: true},
		{name: "weird-method.bin", data: []byte("whatever"), method: 99, raw: true},
		{name: "empty.txt", method: zip.Deflate},
		{name: "last.txt", data: []byte("the end"), method: zip.Store},
	}
	got, order, err := readTestZip(buildTestZip(t, entries), 1<<30)
	if err != nil {
		t.Fatalf("reading archive: %v", err)
	}
	if len(order) != len(entries) {
		t.Fatalf("read entries %q, want %d entries", order, len(entries))
	}
	for i, e := range entries {
		if order[i] != e.name {
			t.Errorf("entry %d is %q, want %q", i, order[i], e.name)
		}
		want := e.data
		if e.method == 99 {
			want = nil
		} else if want == nil {
			want = []byte{}
		}
		if g := got[e.name]; !bytes.Equal(g, want) || (g == nil) != (want == nil) {
			t.Errorf("entry %q has %d bytes, want %d", e.name, len(g), len(want))
		}
	}
}

func TestZipStreamSkipsUnreadEntries(t *testing.T) {
	archive := buildTestZip(t, []testZipEntry{
		{name: "a", data: bytes.Repeat([]byte("a"), 10000), method: zip.Deflate},
		{name: "b", data: []byte("PK\x07\x08 b data"), method: zip.Store},
		{name: "c", data: []byte("c data"), method: zip.Store, raw: true},
	})
	zs := newZipStream(bytes.NewReader(archive), 1<<30)
	var names []string
	for {
		e, err := zs.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		names = append(names, e.Name)
		// Read only a little of each.
		e.Read(make([]byte, 3))
	}
	if got, want := strings.Join(names, ","), "a,b,c"; got != want {
		t.Errorf("entries = %q, want %q", got, want)
	}
}

func TestZipStreamErrors(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		archive  func(t *testing.T) []byte
		maxBytes int64
		want     error
	}{
		{
			desc: "bad checksum",
			archive: func(t *testing.T) []byte {
				return buildTestZip(t, []testZipEntry{{name: "a", data: []byte("hello"), method: zip.Store, raw: true, badCRC: true}})
			},
			maxBytes: 1 << 30,
			want:     errZipChecksum,
		},
		{
			desc: "too large",
			archive: func(t *testing.T) []byte {
				return buildTestZip(t, []testZipEntry{
					{name: "a", data: make([]byte, 600), method: zip.Deflate},
					{name: "b", data: make([]byte, 600), method: zip.Deflate},
				})
			},
			maxBytes: 1000,
			want:     errZipTooLarge,
		},
		{
			desc: "truncated",
			archive: func(t *testing.T) []byte {
				data := make([]byte, 10000)
				rand.NewChaCha8([32]byte{}).Read(data)
				a := buildTestZip(t, []testZipEntry{{name: "a", data: data, method: zip.Deflate}})
				return a[:5000]
			},
			maxBytes: 1 << 30,
			want:     io.ErrUnexpectedEOF,
		},
		{
			desc:     "not a zip",
			archive:  func(t *testing.T) []byte { return []byte("this is not a zip file at all") },
			maxBytes: 1 << 30,
			want:     errZipFormat,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, err := readTestZip(tc.archive(t), tc.maxBytes)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
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
