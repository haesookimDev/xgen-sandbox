package k8s

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// PodInfo holds cached information about a sandbox pod.
type PodInfo struct {
	SandboxID string
	PodName   string
	PodIP     string
	Phase     corev1.PodPhase
	Ready     bool
}

// PodManager handles K8s pod lifecycle for sandboxes.
type PodManager struct {
	client      kubernetes.Interface
	namespace   string
	sidecar     string
	runtime     string
	pullPolicy  corev1.PullPolicy

	mu   sync.RWMutex
	pods map[string]*PodInfo // sandboxID -> PodInfo

	onReady func(sandboxID string)
}

// NewPodManagerWithClient creates a PodManager with an injected Kubernetes client (for testing).
func NewPodManagerWithClient(client kubernetes.Interface, namespace, sidecarImage, runtimeImage string, pullPolicy corev1.PullPolicy, onReady func(string)) *PodManager {
	return &PodManager{
		client:     client,
		namespace:  namespace,
		sidecar:    sidecarImage,
		runtime:    runtimeImage,
		pullPolicy: pullPolicy,
		pods:       make(map[string]*PodInfo),
		onReady:    onReady,
	}
}

// NewPodManager creates a new PodManager.
func NewPodManager(namespace, sidecarImage, runtimeImage, imagePullPolicy string, onReady func(string)) (*PodManager, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig for local dev
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, fmt.Errorf("kubernetes config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}

	pm := &PodManager{
		client:     clientset,
		namespace:  namespace,
		sidecar:    sidecarImage,
		runtime:    runtimeImage,
		pullPolicy: corev1.PullPolicy(imagePullPolicy),
		pods:       make(map[string]*PodInfo),
		onReady:    onReady,
	}

	return pm, nil
}

// StartWatcher begins watching sandbox pods for status changes.
func (pm *PodManager) StartWatcher(ctx context.Context) {
	go pm.watchPods(ctx)
}

func (pm *PodManager) watchPods(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		watcher, err := pm.client.CoreV1().Pods(pm.namespace).Watch(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=xgen-sandbox",
		})
		if err != nil {
			log.Printf("pod watch error: %v, retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for event := range watcher.ResultChan() {
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			sandboxID := pod.Labels["xgen.io/sandbox-id"]
			if sandboxID == "" {
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				ready := isPodReady(pod)
				pm.mu.Lock()
				existing := pm.pods[sandboxID]
				wasReady := existing != nil && existing.Ready
				pm.pods[sandboxID] = &PodInfo{
					SandboxID: sandboxID,
					PodName:   pod.Name,
					PodIP:     pod.Status.PodIP,
					Phase:     pod.Status.Phase,
					Ready:     ready,
				}
				pm.mu.Unlock()

				if ready && !wasReady && pm.onReady != nil {
					go pm.onReady(sandboxID)
				}

			case watch.Deleted:
				pm.mu.Lock()
				delete(pm.pods, sandboxID)
				pm.mu.Unlock()
			}
		}
	}
}

func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// CreatePod creates a new sandbox pod.
func (pm *PodManager) CreatePod(ctx context.Context, sandboxID, template string, env map[string]string, ports []int, gui bool) error {
	runtimeImage := pm.runtimeImageForTemplate(template)

	envVars := []corev1.EnvVar{
		{Name: "SANDBOX_ID", Value: sandboxID},
	}
	for k, v := range env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}

	sandboxUser := int64(1000)
	restrictedSC := &corev1.SecurityContext{
		RunAsUser:                &sandboxUser,
		RunAsNonRoot:             boolPtr(true),
		AllowPrivilegeEscalation: boolPtr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}

	rootUser := int64(0)
	containers := []corev1.Container{
		{
			Name:            "sidecar",
			Image:           pm.sidecar,
			ImagePullPolicy: pm.pullPolicy,
			Ports: []corev1.ContainerPort{
				{Name: "ws", ContainerPort: 9000},
				{Name: "health", ContainerPort: 9001},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:              &rootUser,
				ReadOnlyRootFilesystem: boolPtr(true),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
					Add:  []corev1.Capability{"SYS_CHROOT", "SYS_PTRACE"},
				},
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/readyz",
						Port: intstr9001(),
					},
				},
				InitialDelaySeconds: 1,
				PeriodSeconds:       2,
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/home/sandbox/workspace"},
			},
		},
		{
			Name:            "runtime",
			Image:           runtimeImage,
			ImagePullPolicy: pm.pullPolicy,
			Command:         []string{"sleep", "infinity"},
			Env:             envVars,
			SecurityContext: restrictedSC,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/home/sandbox/workspace"},
			},
		},
	}

	// Add VNC container if GUI is requested
	if gui {
		containers = append(containers, corev1.Container{
			Name:            "vnc",
			Image:           "ghcr.io/xgen-sandbox/novnc:latest",
			ImagePullPolicy: pm.pullPolicy,
			SecurityContext: restrictedSC,
			Ports: []corev1.ContainerPort{
				{Name: "novnc", ContainerPort: 6080},
			},
			Env: []corev1.EnvVar{
				{Name: "VNC_RESOLUTION", Value: "1280x720"},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
		})
	}

	shareProcessNamespace := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sbx-" + sandboxID,
			Namespace: pm.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "xgen-sandbox",
				"xgen.io/sandbox-id":    sandboxID,
				"xgen.io/template":      template,
			},
		},
		Spec: corev1.PodSpec{
			ShareProcessNamespace:        &shareProcessNamespace,
			RestartPolicy:                corev1.RestartPolicyNever,
			AutomountServiceAccountToken: boolPtr(false),
			SecurityContext: &corev1.PodSecurityContext{
				FSGroup: &sandboxUser,
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: containers,
			Volumes: []corev1.Volume{
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: resourcePtr(resource.MustParse("1Gi")),
						},
					},
				},
			},
		},
	}

	_, err := pm.client.CoreV1().Pods(pm.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create pod: %w", err)
	}

	return nil
}

// DeletePod deletes a sandbox pod with a 10-second grace period.
func (pm *PodManager) DeletePod(ctx context.Context, sandboxID string) error {
	podName := "sbx-" + sandboxID
	gracePeriod := int64(10)
	err := pm.client.CoreV1().Pods(pm.namespace).Delete(ctx, podName, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
	if err != nil {
		return fmt.Errorf("delete pod: %w", err)
	}

	pm.mu.Lock()
	delete(pm.pods, sandboxID)
	pm.mu.Unlock()

	return nil
}

// ForceDeletePod immediately deletes a sandbox pod with no grace period.
func (pm *PodManager) ForceDeletePod(ctx context.Context, sandboxID string) error {
	podName := "sbx-" + sandboxID
	zero := int64(0)
	err := pm.client.CoreV1().Pods(pm.namespace).Delete(ctx, podName, metav1.DeleteOptions{
		GracePeriodSeconds: &zero,
	})
	if err != nil {
		return fmt.Errorf("force delete pod: %w", err)
	}

	pm.mu.Lock()
	delete(pm.pods, sandboxID)
	pm.mu.Unlock()

	return nil
}

// GetPodInfo returns cached pod info for a sandbox.
func (pm *PodManager) GetPodInfo(sandboxID string) (*PodInfo, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	info, ok := pm.pods[sandboxID]
	return info, ok
}

// RemapPod transfers a pod's cache entry from one sandboxID to another.
func (pm *PodManager) RemapPod(oldID, newID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if info, ok := pm.pods[oldID]; ok {
		info.SandboxID = newID
		pm.pods[newID] = info
		delete(pm.pods, oldID)
	}
}

// ListPods returns all cached sandbox pod infos.
func (pm *PodManager) ListPods() []*PodInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]*PodInfo, 0, len(pm.pods))
	for _, info := range pm.pods {
		result = append(result, info)
	}
	return result
}

func (pm *PodManager) runtimeImageForTemplate(template string) string {
	switch template {
	case "nodejs":
		return "ghcr.io/xgen-sandbox/runtime-nodejs:latest"
	case "python":
		return "ghcr.io/xgen-sandbox/runtime-python:latest"
	case "go":
		return "ghcr.io/xgen-sandbox/runtime-go:latest"
	case "gui":
		return "ghcr.io/xgen-sandbox/runtime-gui:latest"
	default:
		return pm.runtime
	}
}

func boolPtr(b bool) *bool { return &b }
func resourcePtr(r resource.Quantity) *resource.Quantity { return &r }

// intstr9001 returns an IntOrString for port 9001.
func intstr9001() intstr.IntOrString {
	return intstr.FromInt32(9001)
}
