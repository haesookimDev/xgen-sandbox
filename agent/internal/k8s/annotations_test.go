package k8s

import (
	"context"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSandboxAnnotations_RoundTrip(t *testing.T) {
	// RFC3339Nano is the wire format; pre-round to avoid monotonic-clock
	// drift between set and parsed values.
	createdAt, _ := time.Parse(time.RFC3339Nano, "2026-04-22T10:00:00.123456789Z")
	expiresAt, _ := time.Parse(time.RFC3339Nano, "2026-04-22T11:00:00.987654321Z")

	in := SandboxAnnotations{
		Metadata:     map[string]string{"owner": "alice", "ticket": "XYZ-42"},
		Env:          map[string]string{"NODE_ENV": "development", "PORT": "3000"},
		Ports:        []int{3000, 8080},
		GUI:          true,
		Capabilities: []string{"sudo", "git-ssh"},
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
	}

	encoded, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	out := DecodeAnnotations(encoded)
	if !reflect.DeepEqual(out.Metadata, in.Metadata) {
		t.Errorf("metadata: got %v want %v", out.Metadata, in.Metadata)
	}
	if !reflect.DeepEqual(out.Env, in.Env) {
		t.Errorf("env: got %v want %v", out.Env, in.Env)
	}
	if !reflect.DeepEqual(out.Ports, in.Ports) {
		t.Errorf("ports: got %v want %v", out.Ports, in.Ports)
	}
	if out.GUI != in.GUI {
		t.Errorf("gui: got %v want %v", out.GUI, in.GUI)
	}
	if !reflect.DeepEqual(out.Capabilities, in.Capabilities) {
		t.Errorf("capabilities: got %v want %v", out.Capabilities, in.Capabilities)
	}
	if !out.CreatedAt.Equal(in.CreatedAt) {
		t.Errorf("createdAt: got %v want %v", out.CreatedAt, in.CreatedAt)
	}
	if !out.ExpiresAt.Equal(in.ExpiresAt) {
		t.Errorf("expiresAt: got %v want %v", out.ExpiresAt, in.ExpiresAt)
	}
}

func TestSandboxAnnotations_EncodeOmitsZeroFields(t *testing.T) {
	empty := SandboxAnnotations{}
	if !empty.IsZero() {
		t.Error("empty struct should report IsZero=true")
	}
	encoded, err := empty.Encode()
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	if len(encoded) != 0 {
		t.Errorf("expected no annotations for zero input, got %v", encoded)
	}
}

func TestDecodeAnnotations_TolerantOfMalformedFields(t *testing.T) {
	// Mix of good + bad fields. Decode should extract what it can and
	// silently drop the rest — recovery must still make progress.
	in := map[string]string{
		AnnotationMetadata:     `{"owner":"alice"}`,          // valid
		AnnotationPorts:        "3000,garbage,8080",          // partially parseable
		AnnotationGUI:          "not-a-bool",                 // drops to false
		AnnotationCapabilities: "sudo,,git-ssh",              // empty entry skipped
		AnnotationExpiresAt:    "not-an-rfc3339-timestamp",   // drops to zero
	}
	out := DecodeAnnotations(in)

	if out.Metadata["owner"] != "alice" {
		t.Errorf("metadata.owner: got %v", out.Metadata["owner"])
	}
	if !reflect.DeepEqual(out.Ports, []int{3000, 8080}) {
		t.Errorf("ports: expected [3000 8080], got %v", out.Ports)
	}
	if out.GUI {
		t.Error("malformed bool should decay to false")
	}
	if !reflect.DeepEqual(out.Capabilities, []string{"sudo", "git-ssh"}) {
		t.Errorf("capabilities: expected [sudo git-ssh], got %v", out.Capabilities)
	}
	if !out.ExpiresAt.IsZero() {
		t.Error("malformed timestamp should decay to zero Time")
	}
}

func TestCreatePod_WritesSandboxAnnotations(t *testing.T) {
	pm := newTestPodManager()
	ctx := context.Background()

	createdAt := time.Now().Truncate(time.Millisecond)
	expiresAt := createdAt.Add(2 * time.Hour)
	metadata := map[string]string{"owner": "alice"}

	if err := pm.CreatePod(ctx, "ann-id", "nodejs", map[string]string{"PORT": "3000"},
		[]int{3000, 8080}, true, []string{"sudo"},
		metadata, createdAt, expiresAt); err != nil {
		t.Fatalf("CreatePod() error: %v", err)
	}

	pods, _ := pm.client.CoreV1().Pods("test-ns").List(ctx, metav1.ListOptions{})
	if len(pods.Items) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods.Items))
	}
	got := DecodeAnnotations(pods.Items[0].Annotations)
	if got.Metadata["owner"] != "alice" {
		t.Errorf("annotation metadata: got %v", got.Metadata)
	}
	if got.Env["PORT"] != "3000" {
		t.Errorf("annotation env: got %v", got.Env)
	}
	if !reflect.DeepEqual(got.Ports, []int{3000, 8080}) {
		t.Errorf("annotation ports: got %v", got.Ports)
	}
	if !got.GUI {
		t.Error("annotation gui: expected true")
	}
	if !reflect.DeepEqual(got.Capabilities, []string{"sudo"}) {
		t.Errorf("annotation capabilities: got %v", got.Capabilities)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("annotation createdAt: got %v want %v", got.CreatedAt, createdAt)
	}
	if !got.ExpiresAt.Equal(expiresAt) {
		t.Errorf("annotation expiresAt: got %v want %v", got.ExpiresAt, expiresAt)
	}
}

func TestRecoverExistingPods_RoundTripsAnnotationState(t *testing.T) {
	client := fake.NewSimpleClientset()
	pm := NewPodManagerWithClient(client, "test-ns", "sidecar:test", "runtime-base:test", corev1.PullNever, nil)
	ctx := context.Background()

	createdAt := time.Now().Add(-30 * time.Minute).Truncate(time.Millisecond)
	expiresAt := time.Now().Add(30 * time.Minute).Truncate(time.Millisecond)

	if err := pm.CreatePod(ctx, "rec-id", "python",
		map[string]string{"FOO": "bar"},
		[]int{5000}, false, []string{"sudo"},
		map[string]string{"owner": "bob"}, createdAt, expiresAt); err != nil {
		t.Fatalf("CreatePod() error: %v", err)
	}

	// Simulate agent restart: fresh PodManager, same backing client.
	pm2 := NewPodManagerWithClient(client, "test-ns", "sidecar:test", "runtime-base:test", corev1.PullNever, nil)
	recovered, err := pm2.RecoverExistingPods(ctx)
	if err != nil {
		t.Fatalf("RecoverExistingPods() error: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recovered sandbox, got %d", len(recovered))
	}
	rs := recovered[0]
	if rs.SandboxID != "rec-id" {
		t.Errorf("SandboxID: got %q", rs.SandboxID)
	}
	if rs.Template != "python" {
		t.Errorf("Template: got %q", rs.Template)
	}
	if rs.Metadata["owner"] != "bob" {
		t.Errorf("Metadata: got %v", rs.Metadata)
	}
	if rs.Env["FOO"] != "bar" {
		t.Errorf("Env: got %v", rs.Env)
	}
	if !reflect.DeepEqual(rs.Ports, []int{5000}) {
		t.Errorf("Ports: got %v", rs.Ports)
	}
	if !reflect.DeepEqual(rs.Capabilities, []string{"sudo"}) {
		t.Errorf("Capabilities: got %v", rs.Capabilities)
	}
	if !rs.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt: got %v want %v", rs.CreatedAt, createdAt)
	}
	if !rs.ExpiresAt.Equal(expiresAt) {
		t.Errorf("ExpiresAt: got %v want %v", rs.ExpiresAt, expiresAt)
	}
}

func TestRecoverExistingPods_FallsBackToCapabilityLabels(t *testing.T) {
	// Pods predating the annotation scheme only carry labels. Recovery
	// must still recover capabilities from xgen.io/cap-* labels.
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sbx-legacy",
			Namespace: "test-ns",
			Labels: map[string]string{
				"app.kubernetes.io/name": "xgen-sandbox",
				"xgen.io/sandbox-id":     "legacy",
				"xgen.io/template":       "base",
				"xgen.io/cap-sudo":       "true",
			},
			// No annotations at all.
		},
	})
	pm := NewPodManagerWithClient(client, "test-ns", "sidecar:test", "runtime-base:test", corev1.PullNever, nil)

	recovered, err := pm.RecoverExistingPods(context.Background())
	if err != nil {
		t.Fatalf("RecoverExistingPods() error: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recovered, got %d", len(recovered))
	}
	if !reflect.DeepEqual(recovered[0].Capabilities, []string{"sudo"}) {
		t.Errorf("legacy pod capabilities: expected [sudo], got %v", recovered[0].Capabilities)
	}
	if !recovered[0].ExpiresAt.IsZero() {
		t.Errorf("legacy pod ExpiresAt should be zero (caller falls back), got %v", recovered[0].ExpiresAt)
	}
}
