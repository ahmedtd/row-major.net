package main

import "testing"

func TestRendezvousSingle(t *testing.T) {
	if out := rendezvous("abc", []string{"a"}); out != "a" {
		t.Errorf("Bad output from rendezvous with single shard; got %q, want %q", out, "a")
	}
}

func TestRendezvousOrdering(t *testing.T) {
	if rendezvous("abc", []string{"a", "b"}) != rendezvous("abc", []string{"b", "a"}) {
		t.Errorf("Output of rendezvous isn't stable when shards are reordered")
	}
}
