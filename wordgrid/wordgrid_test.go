package wordgrid

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBasic(t *testing.T) {
	words := []string{
		"abcde",
		"bcdea",
		"cdeab",
		"deabc",
		"eabcd",
	}

	evaluator, err := New(words)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	e := evaluator.SubEvaluator(make([]rune, 25))

	wantWords := []string{
		"abcde",
		"bcdea",
		"cdeab",
		"deabc",
		"eabcd",
	}
	for i := 0; i < 5; i++ {
		ok := e.Search(context.Background())
		if !ok {
			t.Fatalf("Didn't yield a solution")
		}

		want := make([]string, 25)
		for r := 0; r < 5; r++ {
			for c := 0; c < 5; c++ {
				want[r*5+c] = string([]rune(wantWords[r])[c])
			}
		}

		sol := e.SolutionAsGrid()
		if diff := cmp.Diff(sol, want); diff != "" {
			t.Fatalf("Bad solution; diff (-got +want)\n%s", diff)
		}

		tmp := wantWords[0]
		wantWords[0] = wantWords[1]
		wantWords[1] = wantWords[2]
		wantWords[2] = wantWords[3]
		wantWords[3] = wantWords[4]
		wantWords[4] = tmp
	}

	ok := e.Search(context.Background())
	if ok {
		t.Fatalf("Yielded an extra solution %+v", e.SolutionAsGrid())
	}
}

func TestRespectsConstraints(t *testing.T) {
	words := []string{
		"abcde",
		"bcdea",
		"cdeab",
		"deabc",
		"eabcd",
	}

	evaluator, err := New(words)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	constraints := []rune{
		'd', 0, 0, 0, 0,
		0, 'a', 0, 0, 0,
		0, 0, 'c', 0, 0,
		0, 0, 0, 'e', 0,
		0, 0, 0, 0, 'b',
	}

	e := evaluator.SubEvaluator(constraints)

	ok := e.Search(context.Background())
	if !ok {
		t.Fatalf("Didn't yield solution, but expected 1 solution")
	}

	want := []string{
		"d", "e", "a", "b", "c",
		"e", "a", "b", "c", "d",
		"a", "b", "c", "d", "e",
		"b", "c", "d", "e", "a",
		"c", "d", "e", "a", "b",
	}

	got := e.SolutionAsGrid()
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("Bad solution, diff (-got +want)\n%s", diff)
	}

	ok = e.Search(context.Background())
	if ok {
		t.Fatalf("Yielded too many solutions")
	}
}

func TestOverConstrained(t *testing.T) {
	words := []string{
		"abcde",
		"bcdea",
		"cdeab",
		"deabc",
		"eabcd",
	}

	evaluator, err := New(words)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	constraints := []rune{
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 'z',
	}

	e := evaluator.SubEvaluator(constraints)

	ok := e.Search(context.Background())
	if ok {
		t.Fatalf("Yielded solution %+v", e.SolutionAsGrid())
	}
}
