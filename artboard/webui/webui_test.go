package webui

import (
	"strings"
	"testing"
)

func TestHumanBytes(t *testing.T) {
	testCases := map[int64]string{
		0:               "0 B",
		1023:            "1023 B",
		1024:            "1.0 KiB",
		1536:            "1.5 KiB",
		5 * 1024 * 1024: "5.0 MiB",
		1 << 40:         "1.0 TiB",
	}
	for in, want := range testCases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestParseByteRange(t *testing.T) {
	testCases := []struct {
		header string
		size   int64
		want   byteRange
		wantOK bool
	}{
		{"bytes=0-99", 1000, byteRange{start: 0, length: 100}, true},
		{"bytes=100-", 1000, byteRange{start: 100, length: 900}, true},
		{"bytes=900-5000", 1000, byteRange{start: 900, length: 100}, true},
		{"bytes=-100", 1000, byteRange{start: 900, length: 100}, true},
		{"bytes=-5000", 1000, byteRange{start: 0, length: 1000}, true},
		{"bytes=999-999", 1000, byteRange{start: 999, length: 1}, true},
		{"bytes=1000-", 1000, byteRange{unsatisfiable: true}, true},
		{"bytes=-0", 1000, byteRange{unsatisfiable: true}, true},
		{"bytes=0-0,5-9", 1000, byteRange{}, false},
		{"bytes=9-5", 1000, byteRange{}, false},
		{"bytes=abc", 1000, byteRange{}, false},
		{"bytes=-", 1000, byteRange{}, false},
		{"items=0-5", 1000, byteRange{}, false},
		{"bytes=0-5", 0, byteRange{}, false},
	}
	for _, tc := range testCases {
		got, ok := parseByteRange(tc.header, tc.size)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("parseByteRange(%q, %d) = %+v, %v; want %+v, %v", tc.header, tc.size, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestEtagListMatches(t *testing.T) {
	testCases := []struct {
		header string
		want   bool
	}{
		{`"123"`, true},
		{`W/"123"`, true},
		{`"456", "123"`, true},
		{`*`, true},
		{`"456"`, false},
	}
	for _, tc := range testCases {
		if got := etagListMatches(tc.header, `"123"`); got != tc.want {
			t.Errorf("etagListMatches(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestNormalizeTag(t *testing.T) {
	testCases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"landscape", "landscape", false},
		{"  two words \t", "two words", false},
		{"", "", true},
		{"   ", "", true},
		{"a\x00b", "", true},
		{"bad\xffutf8", "", true},
		{strings.Repeat("é", maxTagLength), strings.Repeat("é", maxTagLength), false},
		{strings.Repeat("é", maxTagLength+1), "", true},
	}
	for _, tc := range testCases {
		got, err := normalizeTag(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("normalizeTag(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestViewObjectLinkWithTagError(t *testing.T) {
	if got, want := viewObjectLink("abc", "a/b.png"), "/view-object?collection=abc&name=a%2Fb.png"; got != want {
		t.Errorf("viewObjectLink = %q, want %q", got, want)
	}
	got := viewObjectLinkWithTagError("abc", "a/b.png", "Tag must not be empty", " ")
	want := "/view-object?collection=abc&name=a%2Fb.png&tag=+&user-error=Tag+must+not+be+empty#tags"
	if got != want {
		t.Errorf("viewObjectLinkWithTagError = %q, want %q", got, want)
	}
}

func TestShowCollectionSearchLink(t *testing.T) {
	if got, want := showCollectionSearchLink("abc", "red & blue", ""), "/show-collection?id=abc&q=red+%26+blue"; got != want {
		t.Errorf("showCollectionSearchLink = %q, want %q", got, want)
	}
	if got, want := showCollectionSearchLink("abc", "red", "a/b.png"), "/show-collection?after=a%2Fb.png&id=abc&q=red"; got != want {
		t.Errorf("showCollectionSearchLink = %q, want %q", got, want)
	}
}
