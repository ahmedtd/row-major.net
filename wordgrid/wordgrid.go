package wordgrid

import (
	"context"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type trieNode struct {
	children [26]*trieNode
}

func (t *trieNode) add(word []rune) error {
	curNode := t
	for _, r := range word {
		if r < 'a' || r > 'z' {
			return fmt.Errorf("rune %q is not lowercase ASCII", r)
		}

		if curNode.children[r-'a'] == nil {
			curNode.children[r-'a'] = &trieNode{}
		}
		curNode = curNode.children[r-'a']
	}

	return nil
}

func (t *trieNode) containsPrefix(prefix []rune) bool {
	curNode := t
	for _, r := range prefix {
		if r < 'a' || r > 'z' {
			return false
		}

		if curNode.children[r-'a'] == nil {
			return false
		}
		curNode = curNode.children[r-'a']
	}
	return true
}

type Evaluator struct {
	Words        []string
	WordsAsRunes [][]rune
}

func New(words []string) (*Evaluator, error) {
	if len(words) == 0 {
		return nil, fmt.Errorf("must provide at least one word")
	}

	wlen := utf8.RuneCountInString(words[0])
	if wlen != 5 {
		return nil, fmt.Errorf("only words of len 5 are supported")
	}

	for _, w := range words {
		if utf8.RuneCountInString(w) != wlen {
			return nil, fmt.Errorf("word %q has wrong length (want %d)", w, wlen)
		}
	}

	sort.Strings(words)

	wordsAsRunes := [][]rune{}
	for _, w := range words {
		wordsAsRunes = append(wordsAsRunes, []rune(w))
	}

	return &Evaluator{
		Words:        words,
		WordsAsRunes: wordsAsRunes,
	}, nil
}

func NewFromFile(wordsFile string) (*Evaluator, error) {
	wordsData, err := ioutil.ReadFile(wordsFile)
	if err != nil {
		return nil, fmt.Errorf("while reading file %q: %w", wordsFile, err)
	}

	words := strings.Split(string(wordsData), "\n")
	if len(words[len(words)-1]) == 0 {
		words = words[0 : len(words)-1]
	}

	return New(words)
}

func (e *Evaluator) SubEvaluator(constraints []rune) *SubEvaluator {
	rowTries := []*trieNode{
		&trieNode{},
		&trieNode{},
		&trieNode{},
		&trieNode{},
		&trieNode{},
	}
	for r := 0; r < 5; r++ {
	Words1:
		for _, word := range e.WordsAsRunes {
			for c := 0; c < 5; c++ {
				if constraints[r*5+c] != rune(0) && word[c] != constraints[r*5+c] {
					continue Words1
				}
			}
			rowTries[r].add(word)
		}
	}

	return &SubEvaluator{
		evaluator:   e,
		constraints: constraints,

		rowTries: rowTries,

		curCol:         0,
		ColAssignments: []int{-1, -1, -1, -1, -1},
	}
}

type SubEvaluator struct {
	evaluator *Evaluator

	rowTries []*trieNode

	constraints []rune

	curCol         int
	ColAssignments []int
}

func (e *SubEvaluator) areColAssignmentsUnique() bool {
	for i := 0; i < e.curCol-1; i++ {
		for j := i + 1; j < e.curCol; j++ {
			if e.ColAssignments[i] == e.ColAssignments[j] {
				return false
			}
		}
	}
	return true
}

func (e *SubEvaluator) areAllRowsValidWordPrefixes() bool {
	for i := 0; i < 5; i++ {
		prefix := make([]rune, 0, 5)
		for j := 0; j < e.curCol+1; j++ {
			prefix = append(prefix, e.evaluator.WordsAsRunes[e.ColAssignments[j]][i])
		}

		if !e.rowTries[i].containsPrefix(prefix) {
			return false
		}
	}

	return true
}

func (e *SubEvaluator) Search(ctx context.Context) (ok bool) {
	tracer := otel.Tracer("row-major/wordgrid")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "WordGrid SubEvaluator Search")
	defer span.End()

	for e.curCol != -1 {
		e.ColAssignments[e.curCol] += 1

		if e.ColAssignments[e.curCol] == len(e.evaluator.Words) {
			// We have exhausted all choices at this level. Backtrack.
			e.ColAssignments[e.curCol] = -1
			e.curCol -= 1
			continue
		}

		// If we've reused a word, skip this subtree.
		if !e.areColAssignmentsUnique() {
			continue
		}

		// If every row isn't a valid word prefix, skip this subtree.
		if !e.areAllRowsValidWordPrefixes() {
			continue
		}

		// If we are at the bottom level, this is a valid solution.  Emit it.
		if e.curCol == 4 {
			return true
		}

		// If we are not at the bottom level, descend.
		e.curCol += 1
	}

	return false
}

func (e *SubEvaluator) SolutionAsGrid() []string {
	grid := make([]string, 25)
	for r := 0; r < 5; r++ {
		for c := 0; c < 5; c++ {
			grid[r*5+c] = string(e.evaluator.WordsAsRunes[e.ColAssignments[c]][r])
		}
	}
	return grid
}
