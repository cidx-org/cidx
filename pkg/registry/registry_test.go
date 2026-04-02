package registry

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.configPath == "" {
		t.Error("expected configPath to be set")
	}
}

func TestLoadConfig_NoFile(t *testing.T) {
	m := &Manager{configPath: filepath.Join(t.TempDir(), "nonexistent.json")}

	config, err := m.loadConfig()
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if config.Auths == nil {
		t.Error("expected initialized Auths map")
	}
	if len(config.Auths) != 0 {
		t.Error("expected empty Auths map")
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	authStr := base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))
	content := `{
		"auths": {
			"https://index.docker.io/v1/": {"auth": "` + authStr + `"}
		},
		"credsStore": "desktop"
	}`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{configPath: configPath}
	config, err := m.loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.CredsStore != "desktop" {
		t.Errorf("expected credsStore 'desktop', got %q", config.CredsStore)
	}

	auth, ok := config.Auths[DockerHubRegistry]
	if !ok {
		t.Fatal("expected Docker Hub auth entry")
	}
	if auth.Auth != authStr {
		t.Errorf("expected auth %q, got %q", authStr, auth.Auth)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(configPath, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{configPath: configPath}
	_, err := m.loadConfig()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadConfig_NilAuths(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{configPath: configPath}
	config, err := m.loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Auths == nil {
		t.Error("expected Auths to be initialized even if absent from JSON")
	}
}

func TestList_WithAuthEntries(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	authStr := base64.StdEncoding.EncodeToString([]byte("myuser:mypass"))
	content := `{
		"auths": {
			"registry.example.com": {"auth": "` + authStr + `"},
			"empty.example.com": {}
		}
	}`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{configPath: configPath}
	registries, err := m.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(registries) != 2 {
		t.Fatalf("expected 2 registries, got %d", len(registries))
	}

	// Find the authenticated one
	var authed *RegistryInfo
	for i := range registries {
		if registries[i].Name == "registry.example.com" {
			authed = &registries[i]
		}
	}

	if authed == nil {
		t.Fatal("expected registry.example.com in list")
	}
	if !authed.Authenticated {
		t.Error("expected Authenticated=true")
	}
	if authed.Username != "myuser" {
		t.Errorf("expected username 'myuser', got %q", authed.Username)
	}
}

func TestStatus_KnownRegistry(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	authStr := base64.StdEncoding.EncodeToString([]byte("user:token"))
	content := `{"auths": {"ghcr.io": {"auth": "` + authStr + `"}}}`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{configPath: configPath}
	info, err := m.Status("ghcr.io")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !info.Authenticated {
		t.Error("expected authenticated")
	}
	if info.Username != "user" {
		t.Errorf("expected username 'user', got %q", info.Username)
	}
}

func TestStatus_UnknownRegistry(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(configPath, []byte(`{"auths": {}}`), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{configPath: configPath}
	info, err := m.Status("unknown.io")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Authenticated {
		t.Error("expected not authenticated for unknown registry")
	}
}

func TestFormatList_Empty(t *testing.T) {
	output := FormatList(nil)
	if !strings.Contains(output, "No registries configured") {
		t.Error("expected 'No registries configured' message")
	}
}

func TestFormatList_WithEntries(t *testing.T) {
	registries := []RegistryInfo{
		{Name: "ghcr.io", Authenticated: true, Username: "user1"},
		{Name: "dhi.io", Authenticated: false},
	}

	output := FormatList(registries)
	if !strings.Contains(output, "ghcr.io") {
		t.Error("expected ghcr.io in output")
	}
	if !strings.Contains(output, "user1") {
		t.Error("expected user1 in output")
	}
	if !strings.Contains(output, "dhi.io") {
		t.Error("expected dhi.io in output")
	}
}

func TestFormatStatus_Authenticated(t *testing.T) {
	info := &RegistryInfo{
		Name:          "ghcr.io",
		Authenticated: true,
		Username:      "testuser",
		CredsHelper:   "desktop",
	}

	output := FormatStatus(info)
	if !strings.Contains(output, "ghcr.io") {
		t.Error("expected registry name")
	}
	if !strings.Contains(output, "Authenticated") {
		t.Error("expected authenticated status")
	}
	if !strings.Contains(output, "testuser") {
		t.Error("expected username")
	}
	if !strings.Contains(output, "desktop") {
		t.Error("expected credential helper name")
	}
}

func TestFormatStatus_NotAuthenticated(t *testing.T) {
	info := &RegistryInfo{
		Name:          "dhi.io",
		Authenticated: false,
	}

	output := FormatStatus(info)
	if !strings.Contains(output, "Not authenticated") {
		t.Error("expected not authenticated status")
	}
	if !strings.Contains(output, "cidx registry login") {
		t.Error("expected login hint")
	}
}

func TestFormatDHICheck_Ready(t *testing.T) {
	info := &RegistryInfo{
		Name:          DHIRegistry,
		Authenticated: true,
		Username:      "dockeruser",
	}

	output := FormatDHICheck(info)
	if !strings.Contains(output, "DHI is ready") {
		t.Error("expected ready message")
	}
	if !strings.Contains(output, "dockeruser") {
		t.Error("expected username")
	}
}

func TestFormatDHICheck_NotReady(t *testing.T) {
	info := &RegistryInfo{
		Name:          DHIRegistry,
		Authenticated: false,
	}

	output := FormatDHICheck(info)
	if !strings.Contains(output, "requires authentication") {
		t.Error("expected auth required message")
	}
}

func TestIsDHIReady_NoConfig(t *testing.T) {
	m := &Manager{configPath: filepath.Join(t.TempDir(), "nonexistent.json")}
	if m.IsDHIReady() {
		t.Error("expected false when no config exists")
	}
}
