package webui

import "testing"

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
