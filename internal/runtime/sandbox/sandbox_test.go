package sandbox

import (
	"encoding/json"
	"testing"
)

func boolPtr(b bool) *bool                                         { return &b }
func fsModePtr(m FilesystemIsolationMode) *FilesystemIsolationMode { return &m }

func TestResolveRequestWithOverrides(t *testing.T) {
	config := SandboxConfig{
		Enabled:               boolPtr(true),
		NamespaceRestrictions: boolPtr(true),
		NetworkIsolation:      boolPtr(false),
		FilesystemMode:        fsModePtr(FilesystemWorkspaceOnly),
		AllowedMounts:         []string{"logs"},
	}

	request := config.ResolveRequest(
		boolPtr(true),
		boolPtr(false),
		boolPtr(true),
		fsModePtr(FilesystemAllowList),
		[]string{"tmp"},
	)

	if !request.Enabled {
		t.Error("expected enabled")
	}
	if request.NamespaceRestrictions {
		t.Error("expected namespace restrictions off via override")
	}
	if !request.NetworkIsolation {
		t.Error("expected network isolation on via override")
	}
	if request.FilesystemMode != FilesystemAllowList {
		t.Errorf("expected allow-list, got %s", request.FilesystemMode)
	}
	if len(request.AllowedMounts) != 1 || request.AllowedMounts[0] != "tmp" {
		t.Errorf("mounts = %v", request.AllowedMounts)
	}
}

func TestResolveRequestDefaults(t *testing.T) {
	config := SandboxConfig{}
	request := config.ResolveRequest(nil, nil, nil, nil, nil)

	if !request.Enabled {
		t.Error("default enabled should be true")
	}
	if !request.NamespaceRestrictions {
		t.Error("default namespace should be true")
	}
	if request.NetworkIsolation {
		t.Error("default network should be false")
	}
	if request.FilesystemMode != FilesystemWorkspaceOnly {
		t.Errorf("default fs mode = %s", request.FilesystemMode)
	}
}

func TestFilesystemIsolationModeSerialization(t *testing.T) {
	cases := []struct {
		mode     FilesystemIsolationMode
		expected string
	}{
		{FilesystemOff, "off"},
		{FilesystemWorkspaceOnly, "workspace-only"},
		{FilesystemAllowList, "allow-list"},
	}
	for _, tc := range cases {
		data, _ := json.Marshal(tc.mode)
		var got string
		json.Unmarshal(data, &got)
		if got != tc.expected {
			t.Errorf("mode %s serialized to %q, want %q", tc.mode, got, tc.expected)
		}
	}
}

func TestSandboxConfigJSONRoundTrip(t *testing.T) {
	config := SandboxConfig{
		Enabled:               boolPtr(true),
		NamespaceRestrictions: boolPtr(false),
		NetworkIsolation:      boolPtr(true),
		FilesystemMode:        fsModePtr(FilesystemAllowList),
		AllowedMounts:         []string{"/tmp", "logs"},
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}

	var decoded SandboxConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Enabled == nil || *decoded.Enabled != true {
		t.Error("enabled mismatch")
	}
	if decoded.FilesystemMode == nil || *decoded.FilesystemMode != FilesystemAllowList {
		t.Error("filesystem mode mismatch")
	}
}

func TestResolveSandboxStatusFallbackReason(t *testing.T) {
	request := SandboxRequest{
		Enabled:               true,
		NamespaceRestrictions: true,
		NetworkIsolation:      true,
		FilesystemMode:        FilesystemAllowList,
		AllowedMounts:         nil, // empty
	}

	status := ResolveSandboxStatusForRequest(&request, "/workspace")

	// On non-Linux, namespaces are not supported, so there should be fallback reasons
	if !isLinux() {
		if status.FallbackReason == nil {
			t.Error("expected fallback reason on non-Linux")
		}
		if status.Supported {
			t.Error("expected supported = false on non-Linux")
		}
	}
}

func TestBuildLinuxSandboxCommandNilWhenDisabled(t *testing.T) {
	status := &SandboxStatus{
		Enabled:         false,
		NamespaceActive: false,
		NetworkActive:   false,
	}

	cmd := BuildLinuxSandboxCommand("echo hi", "/workspace", status, "/usr/bin")
	if cmd != nil && !isLinux() {
		t.Error("expected nil on non-Linux")
	}
}

func TestBuildLinuxSandboxCommandOnLinux(t *testing.T) {
	if !isLinux() {
		t.Skip("Linux-only test")
	}

	status := &SandboxStatus{
		Enabled:         true,
		NamespaceActive: true,
		NetworkActive:   true,
		FilesystemMode:  FilesystemWorkspaceOnly,
		AllowedMounts:   []string{"/workspace"},
	}

	cmd := BuildLinuxSandboxCommand("printf hi", "/workspace", status, "/usr/bin")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Program != "unshare" {
		t.Errorf("program = %s", cmd.Program)
	}

	hasArg := func(arg string) bool {
		for _, a := range cmd.Args {
			if a == arg {
				return true
			}
		}
		return false
	}
	if !hasArg("--mount") {
		t.Error("missing --mount")
	}
	if !hasArg("--net") {
		t.Error("missing --net when network active")
	}
}

func TestNormalizeMountsAbsoluteAndRelative(t *testing.T) {
	mounts := normalizeMounts([]string{"/abs/path", "rel/path"}, "/workspace")
	if mounts[0] != "/abs/path" {
		t.Errorf("absolute mount = %s", mounts[0])
	}
	if mounts[1] != "/workspace/rel/path" {
		t.Errorf("relative mount = %s", mounts[1])
	}
}
