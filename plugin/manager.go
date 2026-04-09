package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PluginManagerConfig holds discovery configuration.
type PluginManagerConfig struct {
	ConfigHome     string
	EnabledPlugins map[string]bool
	ExternalDirs   []string
	InstallRoot    string // defaults to ConfigHome/plugins/installed
	RegistryPath   string // defaults to ConfigHome/plugins/installed.json
	BundledRoot    string
}

// InstallOutcome records a successful installation.
type InstallOutcome struct {
	PluginID    string
	Version     string
	InstallPath string
}

// InstalledPluginRecord persists info about an installed plugin.
type InstalledPluginRecord struct {
	Kind          PluginKind `json:"kind"`
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Version       string     `json:"version"`
	Description   string     `json:"description"`
	InstallPath   string     `json:"install_path"`
	Source        string     `json:"source"`
	InstalledAtMs int64      `json:"installed_at_unix_ms"`
	UpdatedAtMs   int64      `json:"updated_at_unix_ms"`
}

// InstalledPluginRegistry persists all installed plugin records.
type InstalledPluginRegistry struct {
	Plugins map[string]InstalledPluginRecord `json:"plugins"`
}

// PluginManager handles plugin discovery, installation, and lifecycle.
type PluginManager struct {
	config   PluginManagerConfig
	registry InstalledPluginRegistry
}

// NewPluginManager creates a plugin manager and loads the installed plugin registry.
func NewPluginManager(config PluginManagerConfig) (*PluginManager, error) {
	if config.InstallRoot == "" {
		config.InstallRoot = filepath.Join(config.ConfigHome, "plugins", "installed")
	}
	if config.RegistryPath == "" {
		config.RegistryPath = filepath.Join(config.ConfigHome, "plugins", "installed.json")
	}

	m := &PluginManager{
		config: config,
		registry: InstalledPluginRegistry{
			Plugins: make(map[string]InstalledPluginRecord),
		},
	}

	// Load existing registry if it exists
	if data, err := os.ReadFile(config.RegistryPath); err == nil {
		if err := json.Unmarshal(data, &m.registry); err != nil {
			return nil, &PluginError{
				Kind:    ErrJSON,
				Message: "failed to parse installed plugin registry",
				Cause:   err,
			}
		}
		if m.registry.Plugins == nil {
			m.registry.Plugins = make(map[string]InstalledPluginRecord)
		}
	}

	return m, nil
}

// DiscoverPlugins scans all configured directories for plugins.
func (m *PluginManager) DiscoverPlugins() ([]RegisteredPlugin, []LoadFailure) {
	var plugins []RegisteredPlugin
	var failures []LoadFailure

	// Discover bundled plugins
	if m.config.BundledRoot != "" {
		bp, bf := m.discoverInDir(m.config.BundledRoot, KindBundled, "bundled")
		plugins = append(plugins, bp...)
		failures = append(failures, bf...)
	}

	// Discover installed plugins
	if _, err := os.Stat(m.config.InstallRoot); err == nil {
		ip, ifl := m.discoverInDir(m.config.InstallRoot, KindExternal, "installed")
		plugins = append(plugins, ip...)
		failures = append(failures, ifl...)
	}

	// Discover external plugins from extra dirs
	for _, dir := range m.config.ExternalDirs {
		ep, ef := m.discoverInDir(dir, KindExternal, dir)
		plugins = append(plugins, ep...)
		failures = append(failures, ef...)
	}

	return plugins, failures
}

func (m *PluginManager) discoverInDir(dir string, kind PluginKind, source string) ([]RegisteredPlugin, []LoadFailure) {
	var plugins []RegisteredPlugin
	var failures []LoadFailure

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginRoot := filepath.Join(dir, entry.Name())
		manifestPath := filepath.Join(pluginRoot, "plugin.json")

		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			failures = append(failures, LoadFailure{
				PluginRoot: pluginRoot,
				Kind:       kind,
				Source:     source,
				Err:        err,
			})
			continue
		}

		id := manifest.Name
		enabled := manifest.DefaultEnabled
		if e, ok := m.config.EnabledPlugins[id]; ok {
			enabled = e
		}

		meta := PluginMetadata{
			ID:             id,
			Name:           manifest.Name,
			Version:        manifest.Version,
			Description:    manifest.Description,
			Kind:           kind,
			Source:         source,
			DefaultEnabled: manifest.DefaultEnabled,
			Root:           pluginRoot,
		}

		var p Plugin
		switch kind {
		case KindBundled:
			p = &BundledPlugin{Meta: meta, Manifest: *manifest}
		default:
			p = &ExternalPlugin{Meta: meta, Manifest: *manifest}
		}

		plugins = append(plugins, RegisteredPlugin{Plugin: p, Enabled: enabled})
	}

	return plugins, failures
}

// Install installs a plugin from a source (local path or git URL).
func (m *PluginManager) Install(source string) (*InstallOutcome, error) {
	// Check if source is a git URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") || strings.HasSuffix(source, ".git") {
		return nil, &PluginError{
			Kind:    ErrIO,
			Message: "git-based plugin installation is not yet supported",
		}
	}

	// Local path copy
	source, err := filepath.Abs(source)
	if err != nil {
		return nil, &PluginError{Kind: ErrIO, Message: "invalid source path", Cause: err}
	}

	// Load and validate manifest
	manifestPath := filepath.Join(source, "plugin.json")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	installDir := filepath.Join(m.config.InstallRoot, manifest.Name)

	// Copy plugin directory
	if err := copyDir(source, installDir); err != nil {
		return nil, &PluginError{
			Kind:    ErrIO,
			Message: fmt.Sprintf("failed to copy plugin to %s", installDir),
			Cause:   err,
		}
	}

	now := time.Now().UnixMilli()
	record := InstalledPluginRecord{
		Kind:          KindExternal,
		ID:            manifest.Name,
		Name:          manifest.Name,
		Version:       manifest.Version,
		Description:   manifest.Description,
		InstallPath:   installDir,
		Source:        source,
		InstalledAtMs: now,
		UpdatedAtMs:   now,
	}
	m.registry.Plugins[manifest.Name] = record

	if err := m.saveRegistry(); err != nil {
		return nil, err
	}

	return &InstallOutcome{
		PluginID:    manifest.Name,
		Version:     manifest.Version,
		InstallPath: installDir,
	}, nil
}

// Uninstall removes an installed plugin.
func (m *PluginManager) Uninstall(pluginID string) error {
	record, ok := m.registry.Plugins[pluginID]
	if !ok {
		return &PluginError{
			Kind:    ErrNotFound,
			Message: fmt.Sprintf("plugin %q is not installed", pluginID),
		}
	}

	if err := os.RemoveAll(record.InstallPath); err != nil {
		return &PluginError{
			Kind:    ErrIO,
			Message: fmt.Sprintf("failed to remove plugin directory: %s", record.InstallPath),
			Cause:   err,
		}
	}

	delete(m.registry.Plugins, pluginID)
	return m.saveRegistry()
}

// Enable marks a plugin as enabled in the config.
func (m *PluginManager) Enable(pluginID string) error {
	if m.config.EnabledPlugins == nil {
		m.config.EnabledPlugins = make(map[string]bool)
	}
	m.config.EnabledPlugins[pluginID] = true
	return nil
}

// Disable marks a plugin as disabled in the config.
func (m *PluginManager) Disable(pluginID string) error {
	if m.config.EnabledPlugins == nil {
		m.config.EnabledPlugins = make(map[string]bool)
	}
	m.config.EnabledPlugins[pluginID] = false
	return nil
}

func (m *PluginManager) saveRegistry() error {
	dir := filepath.Dir(m.config.RegistryPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &PluginError{Kind: ErrIO, Message: "failed to create registry directory", Cause: err}
	}

	data, err := json.MarshalIndent(m.registry, "", "  ")
	if err != nil {
		return &PluginError{Kind: ErrJSON, Message: "failed to marshal registry", Cause: err}
	}

	return os.WriteFile(m.config.RegistryPath, data, 0o644)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
