package k8s

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Annotation keys used to persist sandbox state on the K8s Pod.
//
// Labels are reserved for selectors and must stay under 63 chars, so they
// only carry identity (sandbox-id, template, capability flags). Everything
// else — metadata, env, ports, expiry — lives in annotations where the
// values can be arbitrarily large JSON.
//
// These annotations are what lets sandbox state survive an agent restart
// while preserving the original expiry (instead of resetting to now + 1h).
const (
	AnnotationMetadata     = "xgen.io/metadata"     // JSON object
	AnnotationEnv          = "xgen.io/env"          // JSON object
	AnnotationPorts        = "xgen.io/ports"        // CSV of decimal ints
	AnnotationGUI          = "xgen.io/gui"          // "true" | "false"
	AnnotationCapabilities = "xgen.io/capabilities" // CSV
	AnnotationCreatedAt    = "xgen.io/created-at"   // RFC3339Nano (UTC)
	AnnotationExpiresAt    = "xgen.io/expires-at"   // RFC3339Nano (UTC)
)

// SandboxAnnotations is the struct form of the persistent sandbox state.
//
// Zero values mean "absent"; round-trips through Encode/Decode preserve
// all non-empty fields. Decode is intentionally tolerant — a malformed
// individual field falls back to its zero value rather than failing the
// whole recovery.
type SandboxAnnotations struct {
	Metadata     map[string]string
	Env          map[string]string
	Ports        []int
	GUI          bool
	Capabilities []string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// IsZero reports whether the struct carries no information worth persisting.
// Used by pod_manager to decide whether to touch ObjectMeta.Annotations.
func (a SandboxAnnotations) IsZero() bool {
	return len(a.Metadata) == 0 &&
		len(a.Env) == 0 &&
		len(a.Ports) == 0 &&
		!a.GUI &&
		len(a.Capabilities) == 0 &&
		a.CreatedAt.IsZero() &&
		a.ExpiresAt.IsZero()
}

// Encode returns a map ready to drop into ObjectMeta.Annotations.
// Keys that would carry empty/zero values are omitted so we do not
// pollute the pod with noise annotations.
func (a SandboxAnnotations) Encode() (map[string]string, error) {
	out := map[string]string{}

	if len(a.Metadata) > 0 {
		b, err := json.Marshal(a.Metadata)
		if err != nil {
			return nil, err
		}
		out[AnnotationMetadata] = string(b)
	}
	if len(a.Env) > 0 {
		b, err := json.Marshal(a.Env)
		if err != nil {
			return nil, err
		}
		out[AnnotationEnv] = string(b)
	}
	if len(a.Ports) > 0 {
		parts := make([]string, len(a.Ports))
		for i, p := range a.Ports {
			parts[i] = strconv.Itoa(p)
		}
		out[AnnotationPorts] = strings.Join(parts, ",")
	}
	if a.GUI {
		out[AnnotationGUI] = "true"
	}
	if len(a.Capabilities) > 0 {
		out[AnnotationCapabilities] = strings.Join(a.Capabilities, ",")
	}
	if !a.CreatedAt.IsZero() {
		out[AnnotationCreatedAt] = a.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	if !a.ExpiresAt.IsZero() {
		out[AnnotationExpiresAt] = a.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}

	return out, nil
}

// DecodeAnnotations parses a Pod annotation map into SandboxAnnotations.
// Malformed fields decay to zero values; the function never returns an
// error so the agent restart path always makes forward progress.
func DecodeAnnotations(ann map[string]string) SandboxAnnotations {
	var out SandboxAnnotations
	if len(ann) == 0 {
		return out
	}

	if s := ann[AnnotationMetadata]; s != "" {
		_ = json.Unmarshal([]byte(s), &out.Metadata)
	}
	if s := ann[AnnotationEnv]; s != "" {
		_ = json.Unmarshal([]byte(s), &out.Env)
	}
	if s := ann[AnnotationPorts]; s != "" {
		for _, p := range strings.Split(s, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				out.Ports = append(out.Ports, n)
			}
		}
	}
	if s := ann[AnnotationGUI]; s != "" {
		if b, err := strconv.ParseBool(s); err == nil {
			out.GUI = b
		}
	}
	if s := ann[AnnotationCapabilities]; s != "" {
		for _, c := range strings.Split(s, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				out.Capabilities = append(out.Capabilities, c)
			}
		}
	}
	if s := ann[AnnotationCreatedAt]; s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			out.CreatedAt = t
		}
	}
	if s := ann[AnnotationExpiresAt]; s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			out.ExpiresAt = t
		}
	}
	return out
}
