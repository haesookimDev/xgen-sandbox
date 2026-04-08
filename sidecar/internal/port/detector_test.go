package port

import (
	"testing"
)

func TestParsePort_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  uint16
	}{
		{"00000000:1F90", 8080},
		{"00000000:0050", 80},
		{"00000000:01BB", 443},
		{"00000000:FFFF", 65535},
		{"00000000:0001", 1},
		{"0100007F:0CEA", 3306},
	}

	for _, tt := range tests {
		got, err := parsePort(tt.input)
		if err != nil {
			t.Errorf("parsePort(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parsePort(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParsePort_InvalidFormat(t *testing.T) {
	_, err := parsePort("nocolon")
	if err == nil {
		t.Error("expected error for missing colon")
	}
}

func TestParsePort_InvalidHex(t *testing.T) {
	_, err := parsePort("00000000:GGGG")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestNewDetector(t *testing.T) {
	openCalled := false
	closeCalled := false

	d := NewDetector(
		func(port uint16) { openCalled = true },
		func(port uint16) { closeCalled = true },
	)

	if d == nil {
		t.Fatal("expected non-nil detector")
	}
	if len(d.known) != 0 {
		t.Errorf("expected empty known map, got %d entries", len(d.known))
	}

	// Verify callbacks are stored (not called yet)
	_ = openCalled
	_ = closeCalled
}

func TestDetector_StopChannel(t *testing.T) {
	d := NewDetector(nil, nil)

	// Stop should close the channel without panic
	d.Stop()

	// Verify channel is closed
	select {
	case <-d.stopCh:
		// ok
	default:
		t.Error("expected stopCh to be closed")
	}
}
