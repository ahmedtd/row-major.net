package dblayer

import (
	"slices"
	"strings"
)

// searchTrigrams returns the sorted, de-duplicated trigrams of the searchable
// text of an object: its path and its caption.  Trigrams are computed over
// the lower-cased text, one rune at a time, so that search is
// case-insensitive.  The path and caption are trigrammed separately, so no
// trigram spans the boundary between them.
//
// The result is never nil, so that it is stored as an empty array rather than
// null.
func searchTrigrams(path, caption string) []string {
	set := map[string]struct{}{}
	addTrigrams(set, path)
	addTrigrams(set, caption)

	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// addTrigrams adds every trigram of the lower-cased s to set.
func addTrigrams(set map[string]struct{}, s string) {
	runes := []rune(strings.ToLower(s))
	for i := 0; i+3 <= len(runes); i++ {
		set[string(runes[i:i+3])] = struct{}{}
	}
}

// parseSearchQuery splits a search query into lower-cased terms, separated by
// whitespace.
func parseSearchQuery(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// queryTrigrams returns the sorted, de-duplicated trigrams of all the terms.
// Terms shorter than three runes contribute no trigrams.
func queryTrigrams(terms []string) []string {
	set := map[string]struct{}{}
	for _, term := range terms {
		addTrigrams(set, term)
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// matchesAllTerms reports whether every (lower-cased) term appears as a
// substring of the object's path or caption, ignoring case.
func matchesAllTerms(path, caption string, terms []string) bool {
	path = strings.ToLower(path)
	caption = strings.ToLower(caption)
	for _, term := range terms {
		if !strings.Contains(path, term) && !strings.Contains(caption, term) {
			return false
		}
	}
	return true
}
