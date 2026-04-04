package port

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Detector watches for listening ports inside the sandbox.
type Detector struct {
	mu       sync.RWMutex
	known    map[uint16]bool
	onOpen   func(port uint16)
	onClose  func(port uint16)
	stopCh   chan struct{}
	interval time.Duration
}

// NewDetector creates a new port detector.
func NewDetector(onOpen, onClose func(port uint16)) *Detector {
	return &Detector{
		known:    make(map[uint16]bool),
		onOpen:   onOpen,
		onClose:  onClose,
		stopCh:   make(chan struct{}),
		interval: 2 * time.Second,
	}
}

// Start begins polling for port changes.
func (d *Detector) Start() {
	go d.loop()
}

// Stop stops the detector.
func (d *Detector) Stop() {
	close(d.stopCh)
}

func (d *Detector) loop() {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.scan()
		}
	}
}

func (d *Detector) scan() {
	current := d.getListeningPorts()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Detect new ports
	for port := range current {
		if !d.known[port] {
			d.known[port] = true
			if d.onOpen != nil {
				go d.onOpen(port)
			}
		}
	}

	// Detect closed ports
	for port := range d.known {
		if !current[port] {
			delete(d.known, port)
			if d.onClose != nil {
				go d.onClose(port)
			}
		}
	}
}

// getListeningPorts reads /proc/net/tcp and /proc/net/tcp6 for listening sockets.
func (d *Detector) getListeningPorts() map[uint16]bool {
	ports := make(map[uint16]bool)
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		d.parseProcNet(path, ports)
	}
	return ports
}

func (d *Detector) parseProcNet(path string, ports map[uint16]bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// State 0A = TCP_LISTEN
		if fields[3] != "0A" {
			continue
		}

		port, err := parsePort(fields[1])
		if err != nil {
			continue
		}

		// Skip well-known internal ports (sidecar itself)
		if port == 9000 || port == 9001 {
			continue
		}

		ports[port] = true
	}
}

// parsePort extracts the port from a hex-encoded addr:port field (e.g., "00000000:1F90").
func parsePort(addrPort string) (uint16, error) {
	parts := strings.Split(addrPort, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid addr:port: %s", addrPort)
	}
	portHex := parts[1]
	if len(portHex) > 4 {
		// For tcp6, the hex might be different but port is still 4 hex chars
		portHex = portHex[len(portHex)-4:]
	}
	b, err := hex.DecodeString(fmt.Sprintf("%04s", portHex))
	if err != nil {
		return 0, err
	}
	if len(b) < 2 {
		return 0, fmt.Errorf("port hex too short")
	}
	portNum := uint16(b[len(b)-2])<<8 | uint16(b[len(b)-1])

	// Alternative: just parse as int from hex
	n, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil {
		return 0, err
	}
	portNum = uint16(n)

	return portNum, nil
}
