package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- PluginKind tests ---

func TestPluginKindString(t *testing.T) {
	tests := []struct {
		kind PluginKind
		want string
	}{
		{KindBuiltin, "builtin"},
		{KindBundled, "bundled"},
		{KindExternal, "external"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("PluginKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestPluginKindJSONRoundTrip(t *testing.T) {
	for _, kind := range []PluginKind{KindBuiltin, KindBundled, KindExternal} {
		data, err := json.Marshal(kind)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", kind, err)
		}

		var got PluginKind
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", data, err)
		}
		if got != kind {
			t.Errorf("round-trip: got %v, want %v", got, kind)
		}
	}
}

func TestPluginKindJSONValues(t *testing.T) {
	tests := []struct {
		kind PluginKind
		json string
	}{
		{KindBuiltin, `"builtin"`},
		{KindBundled, `"bundled"`},
		{KindExternal, `"external"`},
	}
	for _, tt := range tests {
		data, _ := json.Marshal(tt.kind)
		if string(data) != tt.json {
			t.Errorf("Marshal(%v) = %s, want %s", tt.kind, data, tt.json)
		}
	}
}

func TestPluginKindUnmarshalInvalid(t *testing.T) {
	var k PluginKind
	if err := json.Unmarshal([]byte(`"bogus"`), &k); err == nil {
		t.Error("expected error for invalid kind")
	}
}

// --- Permission string values ---

func TestPluginPermissionValues(t *testing.T) {
	if PermissionRead != "read" {
		t.Errorf("PermissionRead = %q", PermissionRead)
	}
	if PermissionWrite != "write" {
		t.Errorf("PermissionWrite = %q", PermissionWrite)
	}
	if PermissionExecute != "execute" {
		t.Errorf("PermissionExecute = %q", PermissionExecute)
	}
}

func TestPluginToolPermissionValues(t *testing.T) {
	if ToolPermReadOnly != "read-only" {
		t.Errorf("ToolPermReadOnly = %q", ToolPermReadOnly)
	}
	if ToolPermWorkspaceWrite != "workspace-write" {
		t.Errorf("ToolPermWorkspaceWrite = %q", ToolPermWorkspaceWrite)
	}
	if ToolPermDangerFullAccess != "danger-full-access" {
		t.Errorf("ToolPermDangerFullAccess = %q", ToolPermDangerFullAccess)
	}
}

// --- LoadManifest tests ---

func TestLoadManifestValid(t *testing.T) {
	dir := t.TempDir()
	manifestJSON := `{
		"name": "test-plugin",
		"version": "1.0.0",
		"description": "A test plugin",
		"permissions": ["read", "write"],
		"defaultEnabled": true,
		"hooks": {
			"PreToolUse": ["echo pre"],
			"PostToolUse": ["echo post"],
			"PostToolUseFailure": []
		},
		"lifecycle": {
			"Init": ["echo init"],
			"Shutdown": ["echo shutdown"]
		},
		"tools": [
			{
				"name": "my-tool",
				"description": "Does things",
				"inputSchema": {"type": "object"},
				"command": "echo",
				"args": ["hello"],
				"requiredPermission": "read-only"
			}
		],
		"commands": [
			{
				"name": "my-cmd",
				"description": "A command",
				"command": "echo cmd"
			}
		]
	}`
	path := filepath.Join(dir, "plugin.json")
	os.WriteFile(path, []byte(manifestJSON), 0o644)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Name != "test-plugin" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q", m.Version)
	}
	if !m.DefaultEnabled {
		t.Error("DefaultEnabled should be true")
	}
	if len(m.Tools) != 1 {
		t.Fatalf("Tools len = %d", len(m.Tools))
	}
	if m.Tools[0].Name != "my-tool" {
		t.Errorf("Tool name = %q", m.Tools[0].Name)
	}
	if len(m.Hooks.PreToolUse) != 1 {
		t.Errorf("PreToolUse len = %d", len(m.Hooks.PreToolUse))
	}
	if len(m.Commands) != 1 {
		t.Errorf("Commands len = %d", len(m.Commands))
	}
	if m.RawJSON == nil {
		t.Error("RawJSON should be populated")
	}
}

func TestLoadManifestMissingFile(t *testing.T) {
	_, err := LoadManifest("/nonexistent/plugin.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	pe, ok := err.(*PluginError)
	if !ok {
		t.Fatalf("expected *PluginError, got %T", err)
	}
	if pe.Kind != ErrIO {
		t.Errorf("Kind = %q, want %q", pe.Kind, ErrIO)
	}
}

func TestLoadManifestInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	os.WriteFile(path, []byte(`{not json`), 0o644)

	_, err := LoadManifest(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	pe, ok := err.(*PluginError)
	if !ok {
		t.Fatalf("expected *PluginError, got %T", err)
	}
	if pe.Kind != ErrJSON {
		t.Errorf("Kind = %q, want %q", pe.Kind, ErrJSON)
	}
}

// --- ValidateManifest tests ---

func TestValidateEmptyFields(t *testing.T) {
	m := &PluginManifest{}
	errs := ValidateManifest(m, "", KindBuiltin)

	codes := make(map[string]int)
	for _, e := range errs {
		codes[e.Code]++
	}
	if codes["empty_field"] < 3 {
		t.Errorf("expected at least 3 empty_field errors, got %d: %v", codes["empty_field"], errs)
	}
}

func TestValidateDuplicatePermissions(t *testing.T) {
	m := &PluginManifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "test",
		Permissions: []PluginPermission{"read", "read"},
	}
	errs := ValidateManifest(m, "", KindBuiltin)

	found := false
	for _, e := range errs {
		if e.Code == "duplicate_permission" {
			found = true
		}
	}
	if !found {
		t.Error("expected duplicate_permission error")
	}
}

func TestValidateInvalidPermission(t *testing.T) {
	m := &PluginManifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "test",
		Permissions: []PluginPermission{"destroy"},
	}
	errs := ValidateManifest(m, "", KindBuiltin)

	found := false
	for _, e := range errs {
		if e.Code == "invalid_permission" {
			found = true
		}
	}
	if !found {
		t.Error("expected invalid_permission error")
	}
}

func TestValidateToolEmptyName(t *testing.T) {
	m := &PluginManifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "test",
		Tools: []PluginToolManifest{
			{Name: "", Command: "echo"},
		},
	}
	errs := ValidateManifest(m, "", KindBuiltin)

	found := false
	for _, e := range errs {
		if e.Code == "empty_entry_field" {
			found = true
		}
	}
	if !found {
		t.Error("expected empty_entry_field error for tool name")
	}
}

func TestValidateToolEmptyCommand(t *testing.T) {
	m := &PluginManifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "test",
		Tools: []PluginToolManifest{
			{Name: "foo", Command: ""},
		},
	}
	errs := ValidateManifest(m, "", KindBuiltin)

	found := false
	for _, e := range errs {
		if e.Code == "empty_entry_field" {
			found = true
		}
	}
	if !found {
		t.Error("expected empty_entry_field error for tool command")
	}
}

func TestValidateDuplicateToolName(t *testing.T) {
	m := &PluginManifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "test",
		Tools: []PluginToolManifest{
			{Name: "foo", Command: "echo"},
			{Name: "foo", Command: "echo"},
		},
	}
	errs := ValidateManifest(m, "", KindBuiltin)

	found := false
	for _, e := range errs {
		if e.Code == "duplicate_entry" {
			found = true
		}
	}
	if !found {
		t.Error("expected duplicate_entry error")
	}
}

func TestValidateInvalidInputSchema(t *testing.T) {
	m := &PluginManifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "test",
		Tools: []PluginToolManifest{
			{Name: "foo", Command: "echo", InputSchema: json.RawMessage(`"not an object"`)},
		},
	}
	errs := ValidateManifest(m, "", KindBuiltin)

	found := false
	for _, e := range errs {
		if e.Code == "invalid_tool_input_schema" {
			found = true
		}
	}
	if !found {
		t.Error("expected invalid_tool_input_schema error")
	}
}

func TestValidateInvalidToolPermission(t *testing.T) {
	m := &PluginManifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "test",
		Tools: []PluginToolManifest{
			{Name: "foo", Command: "echo", RequiredPermission: "admin"},
		},
	}
	errs := ValidateManifest(m, "", KindBuiltin)

	found := false
	for _, e := range errs {
		if e.Code == "invalid_tool_required_permission" {
			found = true
		}
	}
	if !found {
		t.Error("expected invalid_tool_required_permission error")
	}
}

func TestValidateUnsupportedContracts(t *testing.T) {
	dir := t.TempDir()
	raw := `{
		"name": "test",
		"version": "1.0.0",
		"description": "test",
		"skills": [],
		"mcpServers": {},
		"agents": []
	}`
	path := filepath.Join(dir, "plugin.json")
	os.WriteFile(path, []byte(raw), 0o644)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	errs := ValidateManifest(m, "", KindBuiltin)

	codes := make(map[string]int)
	for _, e := range errs {
		codes[e.Code]++
	}
	if codes["unsupported_contract"] != 3 {
		t.Errorf("expected 3 unsupported_contract errors, got %d: %v", codes["unsupported_contract"], errs)
	}
}

func TestValidateMissingToolPath(t *testing.T) {
	m := &PluginManifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "test",
		Tools: []PluginToolManifest{
			{Name: "foo", Command: "nonexistent-binary-xyz"},
		},
	}
	errs := ValidateManifest(m, t.TempDir(), KindExternal)

	found := false
	for _, e := range errs {
		if e.Code == "missing_path" {
			found = true
		}
	}
	if !found {
		t.Error("expected missing_path error for external plugin")
	}
}

// --- PluginRegistry tests ---

func makeBuiltin(id string, enabled bool, hooks PluginHooks, tools []PluginToolDefinition) RegisteredPlugin {
	return RegisteredPlugin{
		Plugin: &BuiltinPlugin{
			Meta: PluginMetadata{
				ID:   id,
				Name: id,
			},
			HooksDef: hooks,
			ToolsDef: tools,
		},
		Enabled: enabled,
	}
}

func TestRegistrySortsByID(t *testing.T) {
	reg := NewPluginRegistry([]RegisteredPlugin{
		makeBuiltin("charlie", true, PluginHooks{}, nil),
		makeBuiltin("alpha", true, PluginHooks{}, nil),
		makeBuiltin("bravo", true, PluginHooks{}, nil),
	})

	plugins := reg.Plugins()
	if len(plugins) != 3 {
		t.Fatalf("len = %d", len(plugins))
	}
	ids := []string{
		plugins[0].Plugin.Metadata().ID,
		plugins[1].Plugin.Metadata().ID,
		plugins[2].Plugin.Metadata().ID,
	}
	if ids[0] != "alpha" || ids[1] != "bravo" || ids[2] != "charlie" {
		t.Errorf("not sorted: %v", ids)
	}
}

func TestRegistryGetAndContains(t *testing.T) {
	reg := NewPluginRegistry([]RegisteredPlugin{
		makeBuiltin("foo", true, PluginHooks{}, nil),
		makeBuiltin("bar", true, PluginHooks{}, nil),
	})

	if rp := reg.Get("foo"); rp == nil {
		t.Error("Get(foo) returned nil")
	}
	if rp := reg.Get("baz"); rp != nil {
		t.Error("Get(baz) should return nil")
	}
	if !reg.Contains("bar") {
		t.Error("Contains(bar) should be true")
	}
	if reg.Contains("nope") {
		t.Error("Contains(nope) should be false")
	}
}

func TestRegistryAggregatedHooks(t *testing.T) {
	reg := NewPluginRegistry([]RegisteredPlugin{
		makeBuiltin("a", true, PluginHooks{PreToolUse: []string{"cmd1"}}, nil),
		makeBuiltin("b", true, PluginHooks{PreToolUse: []string{"cmd2"}, PostToolUse: []string{"cmd3"}}, nil),
		makeBuiltin("c", false, PluginHooks{PreToolUse: []string{"cmd4"}}, nil), // disabled
	})

	h := reg.AggregatedHooks()
	if len(h.PreToolUse) != 2 {
		t.Errorf("PreToolUse len = %d, want 2", len(h.PreToolUse))
	}
	if len(h.PostToolUse) != 1 {
		t.Errorf("PostToolUse len = %d, want 1", len(h.PostToolUse))
	}
}

func TestRegistryAggregatedToolsConflict(t *testing.T) {
	reg := NewPluginRegistry([]RegisteredPlugin{
		makeBuiltin("a", true, PluginHooks{}, []PluginToolDefinition{
			{Name: "shared-tool", Description: "from a"},
		}),
		makeBuiltin("b", true, PluginHooks{}, []PluginToolDefinition{
			{Name: "shared-tool", Description: "from b"},
			{Name: "unique-tool", Description: "only b"},
		}),
	})

	tools, warnings := reg.AggregatedTools()
	if len(tools) != 3 {
		t.Errorf("tools len = %d, want 3", len(tools))
	}
	if len(warnings) != 1 {
		t.Errorf("warnings len = %d, want 1", len(warnings))
	}
}

func TestRegistryAggregatedToolsDisabledSkipped(t *testing.T) {
	reg := NewPluginRegistry([]RegisteredPlugin{
		makeBuiltin("a", true, PluginHooks{}, []PluginToolDefinition{
			{Name: "tool-a"},
		}),
		makeBuiltin("b", false, PluginHooks{}, []PluginToolDefinition{
			{Name: "tool-b"},
		}),
	})

	tools, warnings := reg.AggregatedTools()
	if len(tools) != 1 {
		t.Errorf("tools len = %d, want 1", len(tools))
	}
	if len(warnings) != 0 {
		t.Errorf("warnings len = %d, want 0", len(warnings))
	}
}

func TestRegistryInitializeShutdownBuiltin(t *testing.T) {
	reg := NewPluginRegistry([]RegisteredPlugin{
		makeBuiltin("a", true, PluginHooks{}, nil),
		makeBuiltin("b", true, PluginHooks{}, nil),
	})

	if err := reg.Initialize(); err != nil {
		t.Errorf("Initialize: %v", err)
	}
	if err := reg.Shutdown(); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestRegistrySummaries(t *testing.T) {
	reg := NewPluginRegistry([]RegisteredPlugin{
		makeBuiltin("x", true, PluginHooks{}, nil),
		makeBuiltin("y", false, PluginHooks{}, nil),
	})

	summaries := reg.Summaries()
	if len(summaries) != 2 {
		t.Fatalf("len = %d", len(summaries))
	}
	if summaries[0].Metadata.ID != "x" || !summaries[0].Enabled {
		t.Errorf("summaries[0] = %+v", summaries[0])
	}
	if summaries[1].Metadata.ID != "y" || summaries[1].Enabled {
		t.Errorf("summaries[1] = %+v", summaries[1])
	}
}

// --- BuiltinPlugin tests ---

func TestBuiltinPluginNoOps(t *testing.T) {
	p := &BuiltinPlugin{
		Meta: PluginMetadata{ID: "builtin-test", Name: "builtin-test"},
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if err := p.Initialize(); err != nil {
		t.Errorf("Initialize: %v", err)
	}
	if err := p.Shutdown(); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

// --- InstalledPluginRegistry JSON round-trip ---

func TestInstalledPluginRegistryRoundTrip(t *testing.T) {
	reg := InstalledPluginRegistry{
		Plugins: map[string]InstalledPluginRecord{
			"my-plugin": {
				Kind:          KindExternal,
				ID:            "my-plugin",
				Name:          "my-plugin",
				Version:       "2.0.0",
				Description:   "A plugin",
				InstallPath:   "/path/to/plugin",
				Source:        "/source",
				InstalledAtMs: 1000,
				UpdatedAtMs:   2000,
			},
		},
	}

	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got InstalledPluginRegistry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	p, ok := got.Plugins["my-plugin"]
	if !ok {
		t.Fatal("missing my-plugin")
	}
	if p.Kind != KindExternal {
		t.Errorf("Kind = %v", p.Kind)
	}
	if p.Version != "2.0.0" {
		t.Errorf("Version = %q", p.Version)
	}
	if p.InstalledAtMs != 1000 {
		t.Errorf("InstalledAtMs = %d", p.InstalledAtMs)
	}
}

// --- PluginError tests ---

func TestPluginErrorWithCause(t *testing.T) {
	cause := &PluginError{Kind: ErrIO, Message: "inner"}
	outer := &PluginError{Kind: ErrJSON, Message: "outer", Cause: cause}

	if outer.Unwrap() != cause {
		t.Error("Unwrap should return cause")
	}
	s := outer.Error()
	if s == "" {
		t.Error("Error() should not be empty")
	}
}

func TestPluginErrorWithoutCause(t *testing.T) {
	e := &PluginError{Kind: ErrNotFound, Message: "not found"}
	if e.Unwrap() != nil {
		t.Error("Unwrap should return nil")
	}
}

func TestValidationErrorString(t *testing.T) {
	e := &ValidationError{Code: "empty_field", Message: "name is required"}
	s := e.Error()
	if s != "[empty_field] name is required" {
		t.Errorf("Error() = %q", s)
	}
}

func TestLoadFailureString(t *testing.T) {
	f := &LoadFailure{
		PluginRoot: "/path",
		Kind:       KindExternal,
		Source:     "dir",
		Err:        &PluginError{Kind: ErrIO, Message: "read failed"},
	}
	s := f.Error()
	if s == "" {
		t.Error("Error() should not be empty")
	}
}

// --- PluginManager tests ---

func TestPluginManagerDiscoverBundled(t *testing.T) {
	dir := t.TempDir()
	bundledRoot := filepath.Join(dir, "bundled")
	pluginDir := filepath.Join(bundledRoot, "my-bundled")
	os.MkdirAll(pluginDir, 0o755)
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
		"name": "my-bundled",
		"version": "1.0.0",
		"description": "bundled plugin",
		"defaultEnabled": true
	}`), 0o644)

	mgr, err := NewPluginManager(PluginManagerConfig{
		ConfigHome:  filepath.Join(dir, "config"),
		BundledRoot: bundledRoot,
	})
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}

	plugins, failures := mgr.DiscoverPlugins()
	if len(failures) != 0 {
		t.Errorf("failures: %v", failures)
	}
	if len(plugins) != 1 {
		t.Fatalf("plugins len = %d", len(plugins))
	}
	if plugins[0].Plugin.Metadata().Name != "my-bundled" {
		t.Errorf("Name = %q", plugins[0].Plugin.Metadata().Name)
	}
	if !plugins[0].Enabled {
		t.Error("should be enabled by default")
	}
}

func TestPluginManagerInstallLocal(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "source-plugin")
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "plugin.json"), []byte(`{
		"name": "local-plugin",
		"version": "0.1.0",
		"description": "a local plugin"
	}`), 0o644)

	configHome := filepath.Join(dir, "config")
	mgr, err := NewPluginManager(PluginManagerConfig{
		ConfigHome: configHome,
	})
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}

	outcome, err := mgr.Install(srcDir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if outcome.PluginID != "local-plugin" {
		t.Errorf("PluginID = %q", outcome.PluginID)
	}
	if outcome.Version != "0.1.0" {
		t.Errorf("Version = %q", outcome.Version)
	}

	// Verify registry persisted
	mgr2, err := NewPluginManager(PluginManagerConfig{
		ConfigHome: configHome,
	})
	if err != nil {
		t.Fatalf("NewPluginManager reload: %v", err)
	}
	if _, ok := mgr2.registry.Plugins["local-plugin"]; !ok {
		t.Error("plugin not in registry after reload")
	}
}

func TestPluginManagerInstallGitUnsupported(t *testing.T) {
	mgr, _ := NewPluginManager(PluginManagerConfig{
		ConfigHome: t.TempDir(),
	})
	_, err := mgr.Install("https://github.com/example/plugin.git")
	if err == nil {
		t.Fatal("expected error for git install")
	}
}

func TestPluginManagerUninstall(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "plugin.json"), []byte(`{
		"name": "rm-me",
		"version": "1.0.0",
		"description": "removable"
	}`), 0o644)

	mgr, _ := NewPluginManager(PluginManagerConfig{ConfigHome: filepath.Join(dir, "config")})
	mgr.Install(srcDir)

	if err := mgr.Uninstall("rm-me"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, ok := mgr.registry.Plugins["rm-me"]; ok {
		t.Error("plugin should be removed from registry")
	}
}

func TestPluginManagerUninstallNotFound(t *testing.T) {
	mgr, _ := NewPluginManager(PluginManagerConfig{ConfigHome: t.TempDir()})
	err := mgr.Uninstall("nonexistent")
	if err == nil {
		t.Fatal("expected error for uninstalling nonexistent plugin")
	}
}

func TestPluginManagerEnableDisable(t *testing.T) {
	mgr, _ := NewPluginManager(PluginManagerConfig{ConfigHome: t.TempDir()})

	mgr.Enable("foo")
	if !mgr.config.EnabledPlugins["foo"] {
		t.Error("foo should be enabled")
	}
	mgr.Disable("foo")
	if mgr.config.EnabledPlugins["foo"] {
		t.Error("foo should be disabled")
	}
}
