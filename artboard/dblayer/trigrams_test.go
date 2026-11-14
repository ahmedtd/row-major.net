package dblayer

import (
	"slices"
	"testing"
)

func TestSearchTrigrams(t *testing.T) {
	testCases := []struct {
		desc          string
		path, caption string
		want          []string
	}{
		{
			desc: "empty",
			want: []string{},
		},
		{
			desc: "short path",
			path: "ab",
			want: []string{},
		},
		{
			desc:    "path and caption are lower-cased, de-duplicated, and do not span the boundary",
			path:    "AbCd",
			caption: "bcdbc",
			want:    []string{"abc", "bcd", "cdb", "dbc"},
		},
		{
			desc: "multi-byte runes",
			path: "héllo",
			want: []string{"hél", "llo", "éll"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := searchTrigrams(tc.path, tc.caption)
			if got == nil {
				t.Errorf("searchTrigrams returned nil, want non-nil")
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("searchTrigrams(%q, %q) = %q, want %q", tc.path, tc.caption, got, tc.want)
			}
		})
	}
}

func TestQueryTrigrams(t *testing.T) {
	terms := parseSearchQuery("  C2B58\tab  ")
	if want := []string{"c2b58", "ab"}; !slices.Equal(terms, want) {
		t.Fatalf("parseSearchQuery = %q, want %q", terms, want)
	}
	got := queryTrigrams(terms)
	if want := []string{"2b5", "b58", "c2b"}; !slices.Equal(got, want) {
		t.Errorf("queryTrigrams(%q) = %q, want %q", terms, got, want)
	}
}

func TestMatchesAllTerms(t *testing.T) {
	testCases := []struct {
		desc    string
		path    string
		caption string
		query   string
		want    bool
	}{
		{"substring of path", "C2B582A.mp4", "", "C2B58", true},
		{"case-insensitive", "C2B582A.mp4", "", "c2b58", true},
		{"terms split across fields", "C2B582A.mp4", "A red fox", "c2b fox", true},
		{"one term missing", "C2B582A.mp4", "A red fox", "c2b dog", false},
		{"trigrams present but not contiguous", "abcxbcd", "", "abcd", false},
		{"empty query", "anything", "", "", true},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if got := matchesAllTerms(tc.path, tc.caption, parseSearchQuery(tc.query)); got != tc.want {
				t.Errorf("matchesAllTerms(%q, %q, %q) = %v, want %v", tc.path, tc.caption, tc.query, got, tc.want)
			}
		})
	}
}
