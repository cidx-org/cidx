package registry

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DHIRegistry is the Docker Hardened Images registry
const DHIRegistry = "dhi.io"

// DockerHubRegistry is the Docker Hub registry URL
const DockerHubRegistry = "https://index.docker.io/v1/"

// DockerConfig represents the Docker config.json structure
type DockerConfig struct {
	Auths      map[string]AuthEntry `json:"auths"`
	CredsStore string               `json:"credsStore,omitempty"`
}

// AuthEntry represents an auth entry in Docker config
type AuthEntry struct {
	Auth string `json:"auth,omitempty"`
}

// RegistryInfo contains information about a configured registry
type RegistryInfo struct {
	Name         string
	Authenticated bool
	Username     string
	CredsHelper  string // e.g., "desktop", "pass", "secretservice"
}

// Manager handles registry operations
type Manager struct {
	configPath string
}

// NewManager creates a new registry manager
func NewManager() *Manager {
	home, _ := os.UserHomeDir()
	return &Manager{
		configPath: filepath.Join(home, ".docker", "config.json"),
	}
}

// List returns all configured registries
func (m *Manager) List() ([]RegistryInfo, error) {
	config, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	var registries []RegistryInfo

	// Check for credential helper
	credsHelper := config.CredsStore

	for name, auth := range config.Auths {
		info := RegistryInfo{
			Name:        name,
			CredsHelper: credsHelper,
		}

		// Check if authenticated
		if auth.Auth != "" {
			info.Authenticated = true
			// Decode username from auth (base64 of user:pass)
			decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) >= 1 {
					info.Username = parts[0]
				}
			}
		} else if credsHelper != "" {
			// Try to get credentials from helper
			info.Authenticated, info.Username = m.checkCredsHelper(credsHelper, name)
		}

		registries = append(registries, info)
	}

	return registries, nil
}

// Status checks the authentication status for a specific registry
func (m *Manager) Status(registryName string) (*RegistryInfo, error) {
	config, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	info := &RegistryInfo{
		Name:        registryName,
		CredsHelper: config.CredsStore,
	}

	// Check in auths
	if auth, ok := config.Auths[registryName]; ok {
		if auth.Auth != "" {
			info.Authenticated = true
			decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) >= 1 {
					info.Username = parts[0]
				}
			}
		}
	}

	// Check credential helper if not found in auths
	if !info.Authenticated && config.CredsStore != "" {
		info.Authenticated, info.Username = m.checkCredsHelper(config.CredsStore, registryName)
	}

	return info, nil
}

// Login wraps docker login command
// For DHI (dhi.io), it automatically reuses Docker Hub credentials if available
func (m *Manager) Login(registryName string) error {
	// For DHI, try to reuse Docker Hub credentials
	if registryName == DHIRegistry {
		if creds := m.GetDockerHubCredentials(); creds != nil {
			fmt.Printf("🔄 Reusing Docker Hub credentials (%s) for DHI...\n", creds.Username)
			return m.loginWithCredentials(registryName, creds)
		}
		fmt.Println("⚠️  No Docker Hub credentials found, falling back to interactive login...")
	}

	// Interactive login
	cmd := exec.Command("docker", "login", registryName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// loginWithCredentials logs into a registry using provided credentials
func (m *Manager) loginWithCredentials(registryName string, creds *Credentials) error {
	cmd := exec.Command("docker", "login", registryName, "-u", creds.Username, "--password-stdin")
	cmd.Stdin = strings.NewReader(creds.Secret)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Logout wraps docker logout command
func (m *Manager) Logout(registryName string) error {
	cmd := exec.Command("docker", "logout", registryName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CheckDHI verifies if DHI (dhi.io) is properly configured
func (m *Manager) CheckDHI() (*RegistryInfo, error) {
	return m.Status(DHIRegistry)
}

// IsDHIReady returns true if DHI is authenticated
func (m *Manager) IsDHIReady() bool {
	info, err := m.CheckDHI()
	if err != nil {
		return false
	}
	return info.Authenticated
}

// loadConfig loads the Docker config.json
func (m *Manager) loadConfig() (*DockerConfig, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &DockerConfig{Auths: make(map[string]AuthEntry)}, nil
		}
		return nil, fmt.Errorf("failed to read Docker config: %w", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse Docker config: %w", err)
	}

	if config.Auths == nil {
		config.Auths = make(map[string]AuthEntry)
	}

	return &config, nil
}

// Credentials holds username and password/token
type Credentials struct {
	Username string `json:"Username"`
	Secret   string `json:"Secret"`
}

// checkCredsHelper checks if credentials exist in a credential helper
func (m *Manager) checkCredsHelper(helper, registry string) (authenticated bool, username string) {
	creds := m.getCredsFromHelper(helper, registry)
	if creds == nil {
		return false, ""
	}
	return creds.Username != "", creds.Username
}

// getCredsFromHelper retrieves full credentials from a credential helper
func (m *Manager) getCredsFromHelper(helper, registry string) *Credentials {
	// Docker credential helper naming convention: docker-credential-<helper>
	helperCmd := fmt.Sprintf("docker-credential-%s", helper)

	cmd := exec.Command(helperCmd, "get")
	cmd.Stdin = strings.NewReader(registry)

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var creds Credentials
	if err := json.Unmarshal(output, &creds); err != nil {
		return nil
	}

	if creds.Username == "" {
		return nil
	}

	return &creds
}

// GetDockerHubCredentials returns Docker Hub credentials if available
func (m *Manager) GetDockerHubCredentials() *Credentials {
	config, err := m.loadConfig()
	if err != nil {
		return nil
	}

	// Check credential helper first
	if config.CredsStore != "" {
		if creds := m.getCredsFromHelper(config.CredsStore, DockerHubRegistry); creds != nil {
			return creds
		}
	}

	// Check direct auth in config
	if auth, ok := config.Auths[DockerHubRegistry]; ok && auth.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
		if err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				return &Credentials{
					Username: parts[0],
					Secret:   parts[1],
				}
			}
		}
	}

	return nil
}

// FormatList formats the registry list for display
func FormatList(registries []RegistryInfo) string {
	if len(registries) == 0 {
		return "No registries configured.\n\nTo add a registry:\n  cidx registry login <registry>\n"
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("REGISTRY                           STATUS      USERNAME\n")
	sb.WriteString("─────────────────────────────────────────────────────────\n")

	for _, r := range registries {
		status := "\033[31m✗ Not authenticated\033[0m"
		if r.Authenticated {
			status = "\033[32m✓ Authenticated\033[0m"
		}

		username := r.Username
		if username == "" {
			username = "-"
		}
		if r.CredsHelper != "" && r.Authenticated {
			username = fmt.Sprintf("%s (via %s)", username, r.CredsHelper)
		}

		fmt.Fprintf(&sb, "%-35s %-20s %s\n", r.Name, status, username)
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatStatus formats a single registry status for display
func FormatStatus(info *RegistryInfo) string {
	var sb strings.Builder
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "Registry: %s\n", info.Name)
	sb.WriteString("─────────────────────────────────\n")

	if info.Authenticated {
		sb.WriteString("Status:   \033[32m✓ Authenticated\033[0m\n")
		fmt.Fprintf(&sb, "Username: %s\n", info.Username)
		if info.CredsHelper != "" {
			fmt.Fprintf(&sb, "Backend:  %s (credential helper)\n", info.CredsHelper)
		}
	} else {
		sb.WriteString("Status:   \033[31m✗ Not authenticated\033[0m\n")
		fmt.Fprintf(&sb, "\nTo authenticate:\n  cidx registry login %s\n", info.Name)
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatDHICheck formats DHI check result for display
func FormatDHICheck(info *RegistryInfo) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("Docker Hardened Images (DHI) Status\n")
	sb.WriteString("────────────────────────────────────\n")

	if info.Authenticated {
		sb.WriteString("\033[32m✓ DHI is ready!\033[0m\n\n")
		fmt.Fprintf(&sb, "  Registry: %s\n", DHIRegistry)
		fmt.Fprintf(&sb, "  Username: %s\n", info.Username)
		sb.WriteString("\n  You can now pull hardened images:\n")
		sb.WriteString("    docker pull dhi.io/trivy:0.68\n")
		sb.WriteString("    cidx run trivy\n")
	} else {
		sb.WriteString("\033[31m✗ DHI requires authentication\033[0m\n\n")
		sb.WriteString("  Docker Hardened Images (dhi.io) requires Docker Hub credentials.\n\n")
		sb.WriteString("  To authenticate:\n")
		sb.WriteString("    cidx registry login dhi.io\n\n")
		sb.WriteString("  This will use your Docker Hub username and password.\n")
		sb.WriteString("  DHI is free and included with any Docker Hub account.\n")
	}

	sb.WriteString("\n")
	return sb.String()
}
