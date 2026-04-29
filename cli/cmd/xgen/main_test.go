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

func TestSessionPolicyPredicates(t *testing.T) {
	policy := sessionPolicy{IdleTTLMS: 1000, KeepaliveAfterMS: 500}
	rec := sessionRecord{LastUsedAtMs: 1000, ExpiresAtMs: 2500}

	if sessionIdleExpired(rec, policy, 1500) {
		t.Fatal("session should not be idle-expired yet")
	}
	if !sessionIdleExpired(rec, policy, 2501) {
		t.Fatal("session should be idle-expired")
	}
	if sessionNeedsKeepalive(rec, policy, 1900) {
		t.Fatal("session should not need keepalive yet")
	}
	if !sessionNeedsKeepalive(rec, policy, 2000) {
		t.Fatal("session should need keepalive inside threshold")
	}
}

func TestSessionFromSandboxPreservesRegistryFields(t *testing.T) {
	rec := sessionFromSandbox("sess_test", sandboxInfo{
		ID:           "sbx-1",
		Template:     "nodejs",
		PreviewURLs:  map[int]string{3000: "http://example.test"},
		Capabilities: []string{"browser"},
		CreatedAtMs:  100,
		ExpiresAtMs:  200,
		Metadata:     map[string]string{"xgen_cwd": "/workspace"},
	})

	if rec.SessionID != "sess_test" || rec.SandboxID != "sbx-1" {
		t.Fatalf("unexpected ids: %#v", rec)
	}
	if rec.Cwd != "/workspace" {
		t.Fatalf("expected cwd from metadata, got %q", rec.Cwd)
	}
	if len(rec.Ports) != 1 || rec.Ports[0] != 3000 {
		t.Fatalf("expected port 3000, got %#v", rec.Ports)
	}
}
