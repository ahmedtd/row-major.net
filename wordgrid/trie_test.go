package wordgrid

import "testing"

func TestTrieSingle(t *testing.T) {
	root := &trieNode{}

	root.add([]rune("a"))

	if !root.containsPrefix([]rune("a")) {
		t.Errorf("Expected trie to have prefix %q", "a")
	}
}

func TestTrieDouble(t *testing.T) {
	root := &trieNode{}

	root.add([]rune("ab"))

	if !root.containsPrefix([]rune("a")) {
		t.Errorf("Expected trie to have prefix %q", "a")
	}

	if !root.containsPrefix([]rune("ab")) {
		t.Errorf("Expected trie to have prefix %q", "a")
	}
}

func TestTrieMultiple(t *testing.T) {
	words := []string{
		"abcde",
		"fghij",
		"klmno",
	}

	root := &trieNode{}

	for _, w := range words {
		root.add([]rune(w))
	}

	for _, w := range words {
		for i := 0; i < len(w); i++ {
			if !root.containsPrefix([]rune(w[0 : i+1])) {
				t.Errorf("Expected trie to contain prefix %q", w[0:i+1])
			}
		}
	}
}
