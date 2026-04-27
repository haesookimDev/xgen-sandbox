package xgen

import (
	"testing"

	"github.com/xgen-sandbox/sdk-go/protocol"
)

func TestDecodeExecExitCode(t *testing.T) {
	payload, err := protocol.EncodePayload(map[string]int{"exit_code": 17})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeExecExitCode(payload); got != 17 {
		t.Fatalf("expected exit_code 17, got %d", got)
	}
}

func TestDecodeExecExitCodeCompat(t *testing.T) {
	payload, err := protocol.EncodePayload(map[string]int{"exitCode": 23})
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeExecExitCode(payload); got != 23 {
		t.Fatalf("expected exitCode 23, got %d", got)
	}
}
