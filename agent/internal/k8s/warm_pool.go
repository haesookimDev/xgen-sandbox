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
	size     int                 // default target pool size per template
	sizes    map[string]int      // per-template overrides
	defaults []string            // templates to pre-warm
}

// NewWarmPool creates a warm pool. Size is the default number of warm pods per template.
// perTemplate overrides the default for specific templates.
func NewWarmPool(podMgr *PodManager, size int, perTemplate ...map[string]int) *WarmPool {
	wp := &WarmPool{
		podMgr:   podMgr,
		pool:     make(map[string][]string),
		size:     size,
		sizes:    make(map[string]int),
		defaults: []string{"base"},
	}
	if len(perTemplate) > 0 && perTemplate[0] != nil {
		wp.sizes = perTemplate[0]
		// Add any templates with explicit sizes to defaults
		for tmpl := range wp.sizes {
			found := false
			for _, d := range wp.defaults {
				if d == tmpl {
					found = true
					break
				}
			}
			if !found {
				wp.defaults = append(wp.defaults, tmpl)
			}
		}
	}
	return wp
}

// targetSize returns the target pool size for a given template.
func (wp *WarmPool) targetSize(template string) int {
	if s, ok := wp.sizes[template]; ok {
		return s
	}
	return wp.size
}

// Start pre-creates pods to fill the warm pool.
func (wp *WarmPool) Start(ctx context.Context) {
	if wp.size <= 0 && len(wp.sizes) == 0 {
		return
	}
	for _, tmpl := range wp.defaults {
		if wp.targetSize(tmpl) > 0 {
			wp.fill(ctx, tmpl)
		}
	}
}

// Claim takes a warm pod from the pool for the given template.
// Returns the warm sandboxID or empty string if none available.
// Only returns pods that are confirmed ready in the PodManager.
func (wp *WarmPool) Claim(template string) string {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	ids := wp.pool[template]
	for i, id := range ids {
		info, ok := wp.podMgr.GetPodInfo(id)
		if ok && info.Ready {
			wp.pool[template] = append(ids[:i], ids[i+1:]...)
			return id
		}
	}
	return ""
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
	needed := wp.targetSize(template) - current
	wp.mu.Unlock()

	for i := 0; i < needed; i++ {
		id := fmt.Sprintf("warm-%s", uuid.New().String()[:8])
		if err := wp.podMgr.CreatePod(ctx, id, template, nil, nil, false, nil); err != nil {
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

// WarmPoolDetail holds the status of a single template's warm pool.
type WarmPoolDetail struct {
	Available int
	Target    int
}

// Status returns the current warm pool state for all templates.
func (wp *WarmPool) Status() map[string]WarmPoolDetail {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	result := make(map[string]WarmPoolDetail)
	// Include all default templates
	for _, tmpl := range wp.defaults {
		result[tmpl] = WarmPoolDetail{
			Available: len(wp.pool[tmpl]),
			Target:    wp.targetSize(tmpl),
		}
	}
	// Include any additional templates that have pods
	for tmpl, ids := range wp.pool {
		if _, ok := result[tmpl]; !ok {
			result[tmpl] = WarmPoolDetail{
				Available: len(ids),
				Target:    wp.targetSize(tmpl),
			}
		}
	}
	return result
}
