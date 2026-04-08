package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestPodManager() *PodManager {
	client := fake.NewSimpleClientset()
	return NewPodManagerWithClient(
		client,
		"test-ns",
		"sidecar:test",
		"runtime-base:test",
		corev1.PullNever,
		nil,
	)
}

func TestCreatePod_BasicSpec(t *testing.T) {
	pm := newTestPodManager()
	ctx := context.Background()

	if err := pm.CreatePod(ctx, "test-id", "nodejs", nil, []int{3000}, false); err != nil {
		t.Fatalf("CreatePod() error: %v", err)
	}

	pods, err := pm.client.CoreV1().Pods("test-ns").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods.Items))
	}

	pod := pods.Items[0]
	if pod.Name != "sbx-test-id" {
		t.Errorf("name: expected sbx-test-id, got %q", pod.Name)
	}
	if pod.Labels["xgen.io/sandbox-id"] != "test-id" {
		t.Errorf("label sandbox-id: expected test-id, got %q", pod.Labels["xgen.io/sandbox-id"])
	}
	if pod.Labels["xgen.io/template"] != "nodejs" {
		t.Errorf("label template: expected nodejs, got %q", pod.Labels["xgen.io/template"])
	}

	// Should have 2 containers: sidecar + runtime
	if len(pod.Spec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(pod.Spec.Containers))
	}
	if pod.Spec.Containers[0].Name != "sidecar" {
		t.Errorf("container[0] name: expected sidecar, got %q", pod.Spec.Containers[0].Name)
	}
	if pod.Spec.Containers[1].Name != "runtime" {
		t.Errorf("container[1] name: expected runtime, got %q", pod.Spec.Containers[1].Name)
	}
}

func TestCreatePod_WithGUI(t *testing.T) {
	pm := newTestPodManager()
	ctx := context.Background()

	if err := pm.CreatePod(ctx, "gui-id", "gui", nil, nil, true); err != nil {
		t.Fatalf("CreatePod(gui=true) error: %v", err)
	}

	pods, _ := pm.client.CoreV1().Pods("test-ns").List(ctx, metav1.ListOptions{})
	pod := pods.Items[0]

	// Should have 3 containers: sidecar + runtime + vnc
	if len(pod.Spec.Containers) != 3 {
		t.Fatalf("expected 3 containers for GUI, got %d", len(pod.Spec.Containers))
	}
	if pod.Spec.Containers[2].Name != "vnc" {
		t.Errorf("container[2] name: expected vnc, got %q", pod.Spec.Containers[2].Name)
	}
}

func TestRuntimeImageForTemplate(t *testing.T) {
	pm := newTestPodManager()

	tests := []struct {
		template string
		expected string
	}{
		{"nodejs", "ghcr.io/xgen-sandbox/runtime-nodejs:latest"},
		{"python", "ghcr.io/xgen-sandbox/runtime-python:latest"},
		{"go", "ghcr.io/xgen-sandbox/runtime-go:latest"},
		{"gui", "ghcr.io/xgen-sandbox/runtime-gui:latest"},
		{"unknown", "runtime-base:test"}, // falls back to default runtime
		{"base", "runtime-base:test"},
	}

	for _, tt := range tests {
		got := pm.runtimeImageForTemplate(tt.template)
		if got != tt.expected {
			t.Errorf("runtimeImageForTemplate(%q) = %q, want %q", tt.template, got, tt.expected)
		}
	}
}

func TestDeletePod(t *testing.T) {
	pm := newTestPodManager()
	ctx := context.Background()

	pm.CreatePod(ctx, "del-id", "base", nil, nil, false)

	// Manually add to cache (normally done by watcher)
	pm.mu.Lock()
	pm.pods["del-id"] = &PodInfo{SandboxID: "del-id", PodName: "sbx-del-id"}
	pm.mu.Unlock()

	if err := pm.DeletePod(ctx, "del-id"); err != nil {
		t.Fatalf("DeletePod() error: %v", err)
	}

	// Verify pod is deleted from K8s
	pods, _ := pm.client.CoreV1().Pods("test-ns").List(ctx, metav1.ListOptions{})
	if len(pods.Items) != 0 {
		t.Errorf("expected 0 pods after delete, got %d", len(pods.Items))
	}

	// Verify cache is cleaned
	if _, ok := pm.GetPodInfo("del-id"); ok {
		t.Error("expected pod to be removed from cache")
	}
}

func TestGetPodInfo_CacheHit(t *testing.T) {
	pm := newTestPodManager()

	pm.mu.Lock()
	pm.pods["test-id"] = &PodInfo{
		SandboxID: "test-id",
		PodIP:     "10.0.0.5",
		Ready:     true,
	}
	pm.mu.Unlock()

	info, ok := pm.GetPodInfo("test-id")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if info.PodIP != "10.0.0.5" {
		t.Errorf("PodIP: expected 10.0.0.5, got %q", info.PodIP)
	}
}

func TestGetPodInfo_CacheMiss(t *testing.T) {
	pm := newTestPodManager()

	_, ok := pm.GetPodInfo("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestRemapPod(t *testing.T) {
	pm := newTestPodManager()

	pm.mu.Lock()
	pm.pods["old-id"] = &PodInfo{SandboxID: "old-id", PodIP: "10.0.0.1"}
	pm.mu.Unlock()

	pm.RemapPod("old-id", "new-id")

	if _, ok := pm.GetPodInfo("old-id"); ok {
		t.Error("old-id should be removed after remap")
	}
	info, ok := pm.GetPodInfo("new-id")
	if !ok {
		t.Fatal("expected new-id to exist after remap")
	}
	if info.SandboxID != "new-id" {
		t.Errorf("SandboxID: expected new-id, got %q", info.SandboxID)
	}
}

func TestListPods(t *testing.T) {
	pm := newTestPodManager()

	pm.mu.Lock()
	pm.pods["a"] = &PodInfo{SandboxID: "a"}
	pm.pods["b"] = &PodInfo{SandboxID: "b"}
	pm.mu.Unlock()

	list := pm.ListPods()
	if len(list) != 2 {
		t.Errorf("expected 2 pods, got %d", len(list))
	}
}

func TestIsPodReady(t *testing.T) {
	tests := []struct {
		name   string
		pod    *corev1.Pod
		expect bool
	}{
		{
			name: "ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			},
			expect: true,
		},
		{
			name: "not ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionFalse},
					},
				},
			},
			expect: false,
		},
		{
			name:   "no conditions",
			pod:    &corev1.Pod{},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPodReady(tt.pod); got != tt.expect {
				t.Errorf("isPodReady() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestCreatePod_EnvVars(t *testing.T) {
	pm := newTestPodManager()
	ctx := context.Background()

	env := map[string]string{"FOO": "bar", "BAZ": "qux"}
	if err := pm.CreatePod(ctx, "env-id", "base", env, nil, false); err != nil {
		t.Fatal(err)
	}

	pods, _ := pm.client.CoreV1().Pods("test-ns").List(ctx, metav1.ListOptions{})
	runtimeContainer := pods.Items[0].Spec.Containers[1]

	// Should have SANDBOX_ID + custom env vars
	if len(runtimeContainer.Env) < 3 {
		t.Errorf("expected at least 3 env vars, got %d", len(runtimeContainer.Env))
	}

	hasSandboxID := false
	for _, e := range runtimeContainer.Env {
		if e.Name == "SANDBOX_ID" && e.Value == "env-id" {
			hasSandboxID = true
		}
	}
	if !hasSandboxID {
		t.Error("expected SANDBOX_ID env var")
	}
}
