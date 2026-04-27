package k8s

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WarmPool maintains sets of pre-created pods for fast sandbox startup.
//
// A pool is keyed by (template, sorted-deduped capability set). Claiming
// is an exact-match operation: a request for template="base",
// caps=["sudo"] only draws from the "base/sudo" pool, never from
// "base/". This is a correctness requirement because capabilities affect
// the runtime image, securityContext, and NetworkPolicy layout.
type WarmPool struct {
	podMgr *PodManager
	mu     sync.Mutex
	pool   map[string][]string // poolKey -> list of warm sandboxIDs
	size   int                 // default target pool size
	sizes  map[string]int      // template-level size override (applies to all capsets for that template)
	specs  []poolSpec          // pre-warm specs
}

// poolSpec is a (template, capabilities) pair that defines a distinct
// warm pool. Capabilities are normalised (sorted, deduplicated, empty
// values dropped) so two logically equivalent sets hash to the same key.
type poolSpec struct {
	Template     string
	Capabilities []string
}

// Key is the stable pool identifier. Used as the map key and as the
// "template" label value on warm_pool_* Prometheus metrics (existing
// label name preserved for dashboard compatibility).
func (s poolSpec) Key() string {
	if len(s.Capabilities) == 0 {
		return s.Template
	}
	return s.Template + "/" + strings.Join(s.Capabilities, ",")
}

// DisplayCapabilities returns a defensive copy for admin reporting.
func (s poolSpec) DisplayCapabilities() []string {
	if len(s.Capabilities) == 0 {
		return nil
	}
	out := make([]string, len(s.Capabilities))
	copy(out, s.Capabilities)
	return out
}

// normalizeCapabilities sorts, deduplicates, and drops empty entries so
// that capability sets hash stably regardless of caller order.
func normalizeCapabilities(caps []string) []string {
	if len(caps) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(caps))
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// defaultWarmTemplates lists templates that get a pre-warm baseline
// (empty-caps pool) in addition to any templates mentioned in
// perTemplateSizes.
var defaultWarmTemplates = []string{"base", "nodejs", "python"}

// NewWarmPool creates a warm pool configured by the three orthogonal
// knobs read from the agent config:
//
//   - size: default target pods per pool (0 disables the whole pool)
//   - perTemplateSizes: optional per-template override
//   - capabilitiesToPreWarm: each listed capability produces an
//     additional per-template pool on top of the empty-caps baseline
//
// The combined pre-warm set is {default templates + templates with
// explicit size override} × {empty-caps + each listed capability}.
// `git-ssh` and `browser` are intentionally expensive (per-pod
// NetworkPolicy / 2Gi memory); operators include them here only if the
// usage pattern justifies it.
func NewWarmPool(
	podMgr *PodManager,
	size int,
	perTemplateSizes map[string]int,
	capabilitiesToPreWarm []string,
) *WarmPool {
	if perTemplateSizes == nil {
		perTemplateSizes = map[string]int{}
	}

	// Collect templates: defaults + any mentioned in per-template sizes.
	templateSet := make(map[string]struct{}, len(defaultWarmTemplates))
	templates := make([]string, 0, len(defaultWarmTemplates)+len(perTemplateSizes))
	for _, t := range defaultWarmTemplates {
		templateSet[t] = struct{}{}
		templates = append(templates, t)
	}
	// Stable ordering for additional templates.
	extra := make([]string, 0, len(perTemplateSizes))
	for t := range perTemplateSizes {
		if _, ok := templateSet[t]; !ok {
			extra = append(extra, t)
		}
	}
	sort.Strings(extra)
	templates = append(templates, extra...)

	// Build specs: empty-caps baseline first, then one spec per listed
	// capability. Ordering is deterministic for readability in logs.
	specs := make([]poolSpec, 0, len(templates)*(1+len(capabilitiesToPreWarm)))
	for _, t := range templates {
		specs = append(specs, poolSpec{Template: t})
		for _, c := range capabilitiesToPreWarm {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			specs = append(specs, poolSpec{Template: t, Capabilities: []string{c}})
		}
	}

	return &WarmPool{
		podMgr: podMgr,
		pool:   make(map[string][]string),
		size:   size,
		sizes:  perTemplateSizes,
		specs:  specs,
	}
}

// targetSize returns the target pool size for a given pool spec. Per-
// template overrides apply across all capability sets for that template.
func (wp *WarmPool) targetSize(spec poolSpec) int {
	if s, ok := wp.sizes[spec.Template]; ok {
		return s
	}
	return wp.size
}

// Start pre-creates pods to fill every configured pool.
func (wp *WarmPool) Start(ctx context.Context) {
	if wp.size <= 0 && len(wp.sizes) == 0 {
		return
	}
	for _, spec := range wp.specs {
		if wp.targetSize(spec) > 0 {
			wp.fill(ctx, spec)
		}
	}
}

// Claim takes a warm pod that exactly matches (template, capabilities).
// Capabilities are normalised; the caller does not need to sort or dedupe.
// Returns the warm sandboxID, or empty string when no matching pod is ready.
func (wp *WarmPool) Claim(template string, capabilities []string) string {
	spec := poolSpec{Template: template, Capabilities: normalizeCapabilities(capabilities)}
	key := spec.Key()

	wp.mu.Lock()
	defer wp.mu.Unlock()

	ids := wp.pool[key]
	for i, id := range ids {
		info, ok := wp.podMgr.GetPodInfo(id)
		if ok && info.Ready {
			wp.pool[key] = append(ids[:i], ids[i+1:]...)
			return id
		}
	}
	return ""
}

// Replenish creates a replacement pod after one is claimed for the given
// (template, capabilities). Capabilities are normalised by the callee.
func (wp *WarmPool) Replenish(ctx context.Context, template string, capabilities []string) {
	if wp.size <= 0 && len(wp.sizes) == 0 {
		return
	}
	spec := poolSpec{Template: template, Capabilities: normalizeCapabilities(capabilities)}
	go wp.fill(ctx, spec)
}

// MarkReady is called when a warm pod becomes ready.
//
// poolKey is the serialized pool identifier (the output of poolSpec.Key);
// the agent onReady callback computes this from pod labels so the warm
// pool stays decoupled from label-name conventions.
func (wp *WarmPool) MarkReady(sandboxID, poolKey string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.pool[poolKey] = append(wp.pool[poolKey], sandboxID)
}

// PoolKeyFor computes the pool key for a (template, capabilities) pair.
// Exported so the agent main.go can compute the key from pod labels
// without importing the unexported poolSpec type.
func PoolKeyFor(template string, capabilities []string) string {
	return poolSpec{Template: template, Capabilities: normalizeCapabilities(capabilities)}.Key()
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

func (wp *WarmPool) fill(ctx context.Context, spec poolSpec) {
	key := spec.Key()
	target := wp.targetSize(spec)
	if target <= 0 {
		return
	}

	wp.mu.Lock()
	current := len(wp.pool[key])
	needed := target - current
	wp.mu.Unlock()

	for i := 0; i < needed; i++ {
		id := fmt.Sprintf("warm-%s", uuid.New().String()[:8])
		gui := spec.Template == "gui" || capSet(spec.Capabilities)["browser"]
		// Warm pre-warms carry no persistent state — they take on
		// metadata and expiry only when claimed (see handleCreateSandbox).
		if err := wp.podMgr.CreatePod(
			ctx, id, spec.Template,
			nil, nil, gui, spec.DisplayCapabilities(),
			nil, time.Time{}, time.Time{},
		); err != nil {
			log.Printf("warm pool: failed to create pod for %s: %v", key, err)
			return
		}
		log.Printf("warm pool: created warm pod %s (pool=%s)", id, key)
	}
}

// Size returns the current number of warm pods for a pool. The poolKey
// is the same string returned by PoolKeyFor.
func (wp *WarmPool) Size(poolKey string) int {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return len(wp.pool[poolKey])
}

// WarmPoolDetail holds the status of a single pool.
type WarmPoolDetail struct {
	Template     string
	Capabilities []string
	Available    int
	Target       int
}

// Status returns the current state of every configured pool, keyed by
// the pool key (see PoolKeyFor).
func (wp *WarmPool) Status() map[string]WarmPoolDetail {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	result := make(map[string]WarmPoolDetail, len(wp.specs))
	seen := make(map[string]struct{}, len(wp.specs))

	for _, spec := range wp.specs {
		key := spec.Key()
		seen[key] = struct{}{}
		result[key] = WarmPoolDetail{
			Template:     spec.Template,
			Capabilities: spec.DisplayCapabilities(),
			Available:    len(wp.pool[key]),
			Target:       wp.targetSize(spec),
		}
	}
	// Surface any pool that has pods but is not in the pre-warm spec
	// (e.g. dynamically added by an operator). Target reported as 0.
	for key, ids := range wp.pool {
		if _, ok := seen[key]; ok {
			continue
		}
		result[key] = WarmPoolDetail{
			Template:  key,
			Available: len(ids),
			Target:    0,
		}
	}
	return result
}
