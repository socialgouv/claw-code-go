package sandbox

import "testing"

func TestDetectsContainerMarkersFromMultipleSources(t *testing.T) {
	cgroup := "12:memory:/docker/abc"
	detected := DetectContainerEnvironmentFrom(SandboxDetectionInputs{
		EnvPairs:           []EnvPair{{Key: "container", Value: "docker"}},
		DockerenvExists:    true,
		ContainerenvExists: false,
		Proc1Cgroup:        &cgroup,
	})

	if !detected.InContainer {
		t.Error("expected InContainer = true")
	}

	has := func(marker string) bool {
		for _, m := range detected.Markers {
			if m == marker {
				return true
			}
		}
		return false
	}

	if !has("/.dockerenv") {
		t.Error("missing /.dockerenv marker")
	}
	if !has("env:container=docker") {
		t.Error("missing env:container=docker marker")
	}
	if !has("/proc/1/cgroup:docker") {
		t.Error("missing /proc/1/cgroup:docker marker")
	}
}

func TestNoContainerWhenNoMarkers(t *testing.T) {
	detected := DetectContainerEnvironmentFrom(SandboxDetectionInputs{})
	if detected.InContainer {
		t.Error("expected InContainer = false")
	}
	if len(detected.Markers) != 0 {
		t.Errorf("expected 0 markers, got %d", len(detected.Markers))
	}
}

func TestKubernetesEnvDetected(t *testing.T) {
	detected := DetectContainerEnvironmentFrom(SandboxDetectionInputs{
		EnvPairs: []EnvPair{{Key: "KUBERNETES_SERVICE_HOST", Value: "10.0.0.1"}},
	})
	if !detected.InContainer {
		t.Error("expected InContainer = true for K8s")
	}
}

func TestCgroupMarkersDetected(t *testing.T) {
	cgroup := "11:devices:/kubepods/burstable/pod123"
	detected := DetectContainerEnvironmentFrom(SandboxDetectionInputs{
		Proc1Cgroup: &cgroup,
	})
	if !detected.InContainer {
		t.Error("expected InContainer = true")
	}
	found := false
	for _, m := range detected.Markers {
		if m == "/proc/1/cgroup:kubepods" {
			found = true
		}
	}
	if !found {
		t.Error("missing kubepods cgroup marker")
	}
}

func TestEmptyEnvValueIgnored(t *testing.T) {
	detected := DetectContainerEnvironmentFrom(SandboxDetectionInputs{
		EnvPairs: []EnvPair{{Key: "DOCKER", Value: ""}},
	})
	if detected.InContainer {
		t.Error("empty env value should not trigger container detection")
	}
}
