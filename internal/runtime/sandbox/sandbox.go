// Package sandbox implements Linux namespace-based sandboxing with container
// detection and fallback degradation for non-Linux platforms.
package sandbox

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// FilesystemIsolationMode controls filesystem access restrictions.
type FilesystemIsolationMode string

const (
	FilesystemOff           FilesystemIsolationMode = "off"
	FilesystemWorkspaceOnly FilesystemIsolationMode = "workspace-only"
	FilesystemAllowList     FilesystemIsolationMode = "allow-list"
)

// DefaultFilesystemIsolationMode is the default mode when not specified.
const DefaultFilesystemIsolationMode = FilesystemWorkspaceOnly

// SandboxConfig holds optional sandbox settings from configuration.
type SandboxConfig struct {
	Enabled               *bool                    `json:"enabled,omitempty"`
	NamespaceRestrictions *bool                    `json:"namespaceRestrictions,omitempty"`
	NetworkIsolation      *bool                    `json:"networkIsolation,omitempty"`
	FilesystemMode        *FilesystemIsolationMode `json:"filesystemMode,omitempty"`
	AllowedMounts         []string                 `json:"allowedMounts,omitempty"`
}

// SandboxRequest is the resolved sandbox request with explicit fields.
type SandboxRequest struct {
	Enabled               bool                    `json:"enabled"`
	NamespaceRestrictions bool                    `json:"namespaceRestrictions"`
	NetworkIsolation      bool                    `json:"networkIsolation"`
	FilesystemMode        FilesystemIsolationMode `json:"filesystemMode"`
	AllowedMounts         []string                `json:"allowedMounts"`
}

// ContainerEnvironment describes whether the process is running in a container.
type ContainerEnvironment struct {
	InContainer bool     `json:"in_container"`
	Markers     []string `json:"markers"`
}

// SandboxStatus describes the resolved sandbox state.
type SandboxStatus struct {
	Enabled            bool                    `json:"enabled"`
	Requested          SandboxRequest          `json:"requested"`
	Supported          bool                    `json:"supported"`
	Active             bool                    `json:"active"`
	NamespaceSupported bool                    `json:"namespace_supported"`
	NamespaceActive    bool                    `json:"namespace_active"`
	NetworkSupported   bool                    `json:"network_supported"`
	NetworkActive      bool                    `json:"network_active"`
	FilesystemMode     FilesystemIsolationMode `json:"filesystem_mode"`
	FilesystemActive   bool                    `json:"filesystem_active"`
	AllowedMounts      []string                `json:"allowed_mounts"`
	InContainer        bool                    `json:"in_container"`
	ContainerMarkers   []string                `json:"container_markers"`
	FallbackReason     *string                 `json:"fallback_reason,omitempty"`
}

// SandboxDetectionInputs provides injectable inputs for container detection testing.
type SandboxDetectionInputs struct {
	EnvPairs           []EnvPair
	DockerenvExists    bool
	ContainerenvExists bool
	Proc1Cgroup        *string
}

// EnvPair is a key-value environment variable pair.
type EnvPair struct {
	Key   string
	Value string
}

// LinuxSandboxCommand describes the command to launch a sandboxed process.
type LinuxSandboxCommand struct {
	Program string
	Args    []string
	Env     []EnvPair
}

// ErrSandboxUnavailable is returned on non-Linux platforms.
var ErrSandboxUnavailable = fmt.Errorf("sandbox: namespace isolation unavailable on this platform")

// ResolveRequest resolves a SandboxConfig into a SandboxRequest, applying overrides.
func (c *SandboxConfig) ResolveRequest(
	enabledOverride *bool,
	namespaceOverride *bool,
	networkOverride *bool,
	filesystemModeOverride *FilesystemIsolationMode,
	allowedMountsOverride []string,
) SandboxRequest {
	enabled := true
	if c.Enabled != nil {
		enabled = *c.Enabled
	}
	if enabledOverride != nil {
		enabled = *enabledOverride
	}

	namespace := true
	if c.NamespaceRestrictions != nil {
		namespace = *c.NamespaceRestrictions
	}
	if namespaceOverride != nil {
		namespace = *namespaceOverride
	}

	network := false
	if c.NetworkIsolation != nil {
		network = *c.NetworkIsolation
	}
	if networkOverride != nil {
		network = *networkOverride
	}

	fsMode := DefaultFilesystemIsolationMode
	if c.FilesystemMode != nil {
		fsMode = *c.FilesystemMode
	}
	if filesystemModeOverride != nil {
		fsMode = *filesystemModeOverride
	}

	mounts := c.AllowedMounts
	if allowedMountsOverride != nil {
		mounts = allowedMountsOverride
	}

	return SandboxRequest{
		Enabled:               enabled,
		NamespaceRestrictions: namespace,
		NetworkIsolation:      network,
		FilesystemMode:        fsMode,
		AllowedMounts:         mounts,
	}
}

// DetectContainerEnvironmentFrom checks for container markers using injectable inputs.
func DetectContainerEnvironmentFrom(inputs SandboxDetectionInputs) ContainerEnvironment {
	var markers []string

	if inputs.DockerenvExists {
		markers = append(markers, "/.dockerenv")
	}
	if inputs.ContainerenvExists {
		markers = append(markers, "/run/.containerenv")
	}

	for _, pair := range inputs.EnvPairs {
		normalized := strings.ToLower(pair.Key)
		switch normalized {
		case "container", "docker", "podman", "kubernetes_service_host":
			if pair.Value != "" {
				markers = append(markers, fmt.Sprintf("env:%s=%s", pair.Key, pair.Value))
			}
		}
	}

	if inputs.Proc1Cgroup != nil {
		for _, needle := range []string{"docker", "containerd", "kubepods", "podman", "libpod"} {
			if strings.Contains(*inputs.Proc1Cgroup, needle) {
				markers = append(markers, fmt.Sprintf("/proc/1/cgroup:%s", needle))
			}
		}
	}

	sort.Strings(markers)
	// Dedup
	if len(markers) > 1 {
		j := 0
		for i := 1; i < len(markers); i++ {
			if markers[i] != markers[j] {
				j++
				markers[j] = markers[i]
			}
		}
		markers = markers[:j+1]
	}

	return ContainerEnvironment{
		InContainer: len(markers) > 0,
		Markers:     markers,
	}
}

// ResolveSandboxStatusForRequest resolves the full sandbox status for a request.
// namespaceSupported is determined by the platform-specific unshareWorks function.
func ResolveSandboxStatusForRequest(request *SandboxRequest, cwd string) SandboxStatus {
	container := DetectContainerEnvironment()
	namespaceSupported := unshareSupported()
	networkSupported := namespaceSupported

	filesystemActive := request.Enabled && request.FilesystemMode != FilesystemOff

	var fallbackReasons []string
	if request.Enabled && request.NamespaceRestrictions && !namespaceSupported {
		fallbackReasons = append(fallbackReasons,
			"namespace isolation unavailable (requires Linux with `unshare`)")
	}
	if request.Enabled && request.NetworkIsolation && !networkSupported {
		fallbackReasons = append(fallbackReasons,
			"network isolation unavailable (requires Linux with `unshare`)")
	}
	if request.Enabled && request.FilesystemMode == FilesystemAllowList && len(request.AllowedMounts) == 0 {
		fallbackReasons = append(fallbackReasons,
			"filesystem allow-list requested without configured mounts")
	}

	active := request.Enabled &&
		(!request.NamespaceRestrictions || namespaceSupported) &&
		(!request.NetworkIsolation || networkSupported)

	allowedMounts := normalizeMounts(request.AllowedMounts, cwd)

	var fallbackReason *string
	if len(fallbackReasons) > 0 {
		joined := strings.Join(fallbackReasons, "; ")
		fallbackReason = &joined
	}

	return SandboxStatus{
		Enabled:            request.Enabled,
		Requested:          *request,
		Supported:          namespaceSupported,
		Active:             active,
		NamespaceSupported: namespaceSupported,
		NamespaceActive:    request.Enabled && request.NamespaceRestrictions && namespaceSupported,
		NetworkSupported:   networkSupported,
		NetworkActive:      request.Enabled && request.NetworkIsolation && networkSupported,
		FilesystemMode:     request.FilesystemMode,
		FilesystemActive:   filesystemActive,
		AllowedMounts:      allowedMounts,
		InContainer:        container.InContainer,
		ContainerMarkers:   container.Markers,
		FallbackReason:     fallbackReason,
	}
}

// ResolveSandboxStatus resolves sandbox status from a SandboxConfig with default overrides.
func ResolveSandboxStatus(config *SandboxConfig, cwd string) SandboxStatus {
	request := config.ResolveRequest(nil, nil, nil, nil, nil)
	return ResolveSandboxStatusForRequest(&request, cwd)
}

// BuildLinuxSandboxCommand builds the unshare command for sandboxed execution.
// Returns nil if sandboxing should not be applied.
func BuildLinuxSandboxCommand(command, cwd string, status *SandboxStatus, pathEnv string) *LinuxSandboxCommand {
	if !isLinux() || !status.Enabled || (!status.NamespaceActive && !status.NetworkActive) {
		return nil
	}

	args := []string{
		"--user", "--map-root-user",
		"--mount", "--ipc", "--pid", "--uts", "--fork",
	}
	if status.NetworkActive {
		args = append(args, "--net")
	}
	args = append(args, "sh", "-lc", command)

	sandboxHome := filepath.Join(cwd, ".sandbox-home")
	sandboxTmp := filepath.Join(cwd, ".sandbox-tmp")

	env := []EnvPair{
		{"HOME", sandboxHome},
		{"TMPDIR", sandboxTmp},
		{"CLAWD_SANDBOX_FILESYSTEM_MODE", string(status.FilesystemMode)},
		{"CLAWD_SANDBOX_ALLOWED_MOUNTS", strings.Join(status.AllowedMounts, ":")},
	}
	if pathEnv != "" {
		env = append(env, EnvPair{"PATH", pathEnv})
	}

	return &LinuxSandboxCommand{
		Program: "unshare",
		Args:    args,
		Env:     env,
	}
}

func normalizeMounts(mounts []string, cwd string) []string {
	result := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		if filepath.IsAbs(mount) {
			result = append(result, mount)
		} else {
			result = append(result, filepath.Join(cwd, mount))
		}
	}
	return result
}
