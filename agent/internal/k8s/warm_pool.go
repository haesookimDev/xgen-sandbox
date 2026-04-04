package k8s

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
)

// WarmPool maintains a pool of pre-created pods for fast sandbox startup.
type WarmPool struct {
	podMgr   *PodManager
	mu       sync.Mutex
	pool     map[string][]string // template -> list of warm sandboxIDs
	size     int                 // target pool size per template
	defaults []string            // templates to pre-warm
}

// NewWarmPool creates a warm pool. Size is the target number of warm pods per template.
func NewWarmPool(podMgr *PodManager, size int) *WarmPool {
	return &WarmPool{
		podMgr:   podMgr,
		pool:     make(map[string][]string),
		size:     size,
		defaults: []string{"base"},
	}
}

// Start pre-creates pods to fill the warm pool.
func (wp *WarmPool) Start(ctx context.Context) {
	if wp.size <= 0 {
		return
	}
	for _, tmpl := range wp.defaults {
		wp.fill(ctx, tmpl)
	}
}

// Claim takes a warm pod from the pool for the given template.
// Returns the warm sandboxID or empty string if none available.
func (wp *WarmPool) Claim(template string) string {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	ids := wp.pool[template]
	if len(ids) == 0 {
		return ""
	}

	id := ids[0]
	wp.pool[template] = ids[1:]
	return id
}

// Replenish creates a replacement pod after one is claimed.
func (wp *WarmPool) Replenish(ctx context.Context, template string) {
	if wp.size <= 0 {
		return
	}
	go wp.fill(ctx, template)
}

// MarkReady is called when a warm pod becomes ready.
// It adds the pod to the available pool.
func (wp *WarmPool) MarkReady(sandboxID, template string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.pool[template] = append(wp.pool[template], sandboxID)
}

// IsWarm checks if a sandboxID is a warm pool pod (not yet claimed).
func (wp *WarmPool) IsWarm(sandboxID string) bool {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	for _, ids := range wp.pool {
		for _, id := range ids {
			if id == sandboxID {
				return true
			}
		}
	}
	return false
}

func (wp *WarmPool) fill(ctx context.Context, template string) {
	wp.mu.Lock()
	current := len(wp.pool[template])
	needed := wp.size - current
	wp.mu.Unlock()

	for i := 0; i < needed; i++ {
		id := fmt.Sprintf("warm-%s", uuid.New().String()[:8])
		if err := wp.podMgr.CreatePod(ctx, id, template, nil, nil, false); err != nil {
			log.Printf("warm pool: failed to create pod for %s: %v", template, err)
			return
		}
		log.Printf("warm pool: created warm pod %s (template=%s)", id, template)
	}
}

// Size returns the current number of warm pods for a template.
func (wp *WarmPool) Size(template string) int {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return len(wp.pool[template])
}
