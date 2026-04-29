package main

import "testing"

func TestParseKV(t *testing.T) {
	got, err := parseKV([]string{"agent=codex", "task=cli"})
	if err != nil {
		t.Fatalf("parseKV() error: %v", err)
	}
	if got["agent"] != "codex" || got["task"] != "cli" {
		t.Fatalf("parseKV() = %#v", got)
	}
}

func TestParseKVRejectsInvalidPair(t *testing.T) {
	if _, err := parseKV([]string{"missing-separator"}); err == nil {
		t.Fatal("expected invalid key=value error")
	}
}

func TestTruncatePair(t *testing.T) {
	stdout, stderr, truncated := truncatePair("abcdef", "ghijkl", 8)
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	if stdout != "abcdef" || stderr != "gh" {
		t.Fatalf("truncatePair() stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestTruncatePairKeepsShortOutput(t *testing.T) {
	stdout, stderr, truncated := truncatePair("abc", "de", 8)
	if truncated {
		t.Fatal("expected truncated=false")
	}
	if stdout != "abc" || stderr != "de" {
		t.Fatalf("truncatePair() stdout=%q stderr=%q", stdout, stderr)
	}
}
