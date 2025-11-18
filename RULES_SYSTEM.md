# CIDX Rules System - Embedded Presets Architecture

## Philosophy: KISS (Keep It Simple, Stupid)

**CIDX approach:** 1 YAML file, embedded at build time

```
rules/presets.yaml (200 lines, all tools)
    ↓ go:embed at build
    ↓ Parse at runtime
Standalone binary (zero-config)
```

### Design Principles

1. **Simple to understand** - One source file for all presets
2. **Simple to edit** - YAML is more readable than Go for config
3. **Simple to contribute** - Just edit YAML, no Go knowledge required
4. **Simple to use** - Embedded in binary, no external files needed
5. **Simple to deploy** - Single binary, works out-of-the-box

---

## Architecture

### Directory Structure

```
cidx/
├── rules/
│   ├── presets.yaml           # ← Source of truth (edit this)
│   └── README.md              # Documentation for preset format
├── pkg/
│   └── presets/
│       ├── types.go           # Go structures
│       ├── embedded.go        # go:embed + loader
│       ├── registry.go        # Public API (refactored)
│       └── merge.go           # MergeWith logic
├── scripts/
│   ├── validate-rules.go      # Validate presets.yaml schema
│   ├── export-to-yaml.go      # One-time: migrate from Go to YAML
│   └── add-preset.go          # Helper: add new preset interactively
├── cmd/cidx/
└── ...
```

### Data Flow

```
Developer                Runtime               User
    │                       │                   │
    ├─ Edit                 │                   │
    │  rules/presets.yaml   │                   │
    │                       │                   │
    ├─ go build ────────────┤                   │
    │  (embed YAML)         │                   │
    │                       │                   │
    │                       ├─ Parse YAML       │
    │                       │  at startup        │
    │                       │                   │
    │                       ├─ Load registry ───┤ cidx run trivy
    │                       │                   │
    │                       └─ Execute ─────────┘
```

**Key point:** YAML is embedded into the binary at **build time**, not loaded at runtime.

---

## File Format: `rules/presets.yaml`

### Schema

```yaml
version: "1.0"

presets:
  <tool_name>:
    name: string # Tool identifier
    phase: string # Phase: security, code, test, build
    image: string # Docker image with version tag (use semantic versions, not :latest)
    command: string # Command to run in container
    workdir: string # Working directory in container
    volumes: array[string] # Volume mounts (supports ${WORKSPACE})
    env: map[string]string # Environment variables (optional)
    config_files: array[string] # Config files to look for (optional)
    options: map[string]Option # Configurable options (optional)

  # Options schema
  <option_name>:
    type: string # Type: string, int, bool, array
    default: any # Default value
    description: string # Help text
    env_var: string # Maps to environment variable (optional)
    flag: string # Maps to command flag (optional)
    values: array[string] # Enum values (optional)
```

### Complete Example

```yaml
version: "1.0"

presets:
  # ============================================================================
  # SECURITY TOOLS
  # ============================================================================

  trivy:
    name: trivy
    phase: security
    image: aquasec/trivy:0.57.1
    command: trivy fs /scan
    workdir: /scan
    volumes:
      - "${WORKSPACE}:/scan"
    options:
      severity:
        type: string
        default: "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"
        flag: "--severity"
        description: "Severities to report (comma-separated)"
      exit_code:
        type: int
        default: 0
        flag: "--exit-code"
        description: "Exit code when vulnerabilities found"
    config_files:
      - trivy.yaml
      - .trivyignore

  gitleaks:
    name: gitleaks
    phase: security
    image: zricethezav/gitleaks:v8.20.1
    command: gitleaks detect --source . --verbose
    workdir: /repo
    volumes:
      - "${WORKSPACE}:/repo"
    config_files:
      - .gitleaks.toml

  # ============================================================================
  # CODE QUALITY TOOLS
  # ============================================================================

  megalinter:
    name: megalinter
    phase: code
    image: oxsecurity/megalinter:v8.2.0
    command: mega-linter
    workdir: /tmp/lint
    volumes:
      - "${WORKSPACE}:/tmp/lint"
    env:
      DEFAULT_WORKSPACE: /tmp/lint
    options:
      flavor:
        type: string
        default: all
        env_var: MEGALINTER_FLAVOR
        description: "Linter flavor (python, ansible, go, etc.)"
        values:
          - all
          - ansible
          - python
          - go
          - javascript
          - terraform
    config_files:
      - .mega-linter.yml
      - megalinter.yml

  prettier:
    name: prettier
    phase: code
    image: tmknom/prettier:3.3.3
    command: prettier --check .
    workdir: /work
    volumes:
      - "${WORKSPACE}:/work"
    options:
      write:
        type: bool
        default: false
        flag: "--write"
        description: "Write formatted files instead of check"
    config_files:
      - .prettierrc
      - .prettierrc.json
      - .prettierrc.yml
      - prettier.config.js

  commitlint:
    name: commitlint
    phase: code
    image: node:20-alpine
    command: sh -c 'npm install -g @commitlint/cli @commitlint/config-conventional && commitlint --from ${FROM} --to ${TO}'
    workdir: /app
    volumes:
      - "${WORKSPACE}:/app"
      - "${WORKSPACE}/.git:/app/.git"
    env:
      FROM: origin/main
      TO: HEAD
    config_files:
      - .commitlintrc.json
      - .commitlintrc.yml
      - commitlint.config.js

  ansible-lint:
    name: ansible-lint
    phase: code
    image: quay.io/ansible/creator-ee:v24.10.0
    command: ansible-lint
    workdir: /work
    volumes:
      - "${WORKSPACE}:/work"
    config_files:
      - .ansible-lint

  # ============================================================================
  # TEST TOOLS
  # ============================================================================

  molecule:
    name: molecule
    phase: test
    image: quay.io/ansible/molecule:latest
    command: molecule test -s ${SCENARIO}
    workdir: /work
    volumes:
      - "${WORKSPACE}:/work"
      - /var/run/docker.sock:/var/run/docker.sock
    env:
      SCENARIO: default
    options:
      scenario:
        type: string
        default: default
        env_var: SCENARIO
        description: "Molecule scenario to run"
```

---

## Implementation

### 1. Go Structures (`pkg/presets/types.go`)

```go
package presets

// Preset defines a complete tool configuration with sensible defaults
type Preset struct {
    Name        string            `yaml:"name" toml:"name"`
    Phase       string            `yaml:"phase" toml:"phase"`
    Image       string            `yaml:"image" toml:"image"`
    Command     string            `yaml:"command" toml:"command"`
    Workdir     string            `yaml:"workdir" toml:"workdir"`
    Volumes     []string          `yaml:"volumes" toml:"volumes"`
    Env         map[string]string `yaml:"env,omitempty" toml:"env,omitempty"`
    ConfigFiles []string          `yaml:"config_files,omitempty" toml:"config_files,omitempty"`
    Options     map[string]Option `yaml:"options,omitempty" toml:"options,omitempty"`
}

// Option defines a configurable parameter for a preset
type Option struct {
    Type        string      `yaml:"type" toml:"type"`
    Default     interface{} `yaml:"default" toml:"default"`
    Description string      `yaml:"description" toml:"description"`
    EnvVar      string      `yaml:"env_var,omitempty" toml:"env_var,omitempty"`
    CommandFlag string      `yaml:"flag,omitempty" toml:"flag,omitempty"`
    Values      []string    `yaml:"values,omitempty" toml:"values,omitempty"`
}

// RulesFile represents the structure of the YAML file
type RulesFile struct {
    Version string             `yaml:"version"`
    Presets map[string]Preset  `yaml:"presets"`
}

// MergeWith merges user overrides into the preset
func (p *Preset) MergeWith(overrides map[string]interface{}) *Preset {
    merged := *p

    if image, ok := overrides["image"].(string); ok {
        merged.Image = image
    }
    if command, ok := overrides["command"].(string); ok {
        merged.Command = command
    }
    if workdir, ok := overrides["workdir"].(string); ok {
        merged.Workdir = workdir
    }
    if volumes, ok := overrides["volumes"].([]string); ok {
        merged.Volumes = volumes
    }
    if env, ok := overrides["env"].(map[string]string); ok {
        if merged.Env == nil {
            merged.Env = make(map[string]string)
        }
        for k, v := range env {
            merged.Env[k] = v
        }
    }

    // Merge options with preset options
    for optName, optValue := range overrides {
        if opt, exists := merged.Options[optName]; exists {
            merged = applyOption(&merged, optName, opt, optValue)
        }
    }

    return &merged
}

// applyOption applies a specific option value to the preset
func applyOption(preset *Preset, name string, opt Option, value interface{}) Preset {
    p := *preset

    // Apply to environment variable if specified
    if opt.EnvVar != "" {
        if p.Env == nil {
            p.Env = make(map[string]string)
        }
        p.Env[opt.EnvVar] = toString(value)
    }

    // Apply to command flag if specified
    if opt.CommandFlag != "" {
        p.Command = p.Command + " " + opt.CommandFlag + " " + toString(value)
    }

    return p
}

// toString converts interface{} to string
func toString(v interface{}) string {
    switch val := v.(type) {
    case string:
        return val
    case int:
        return fmt.Sprintf("%d", val)
    case bool:
        if val {
            return "true"
        }
        return "false"
    default:
        return fmt.Sprintf("%v", v)
    }
}
```

### 2. Embedded Loader (`pkg/presets/embedded.go`)

```go
package presets

import (
    _ "embed"
    "fmt"

    "gopkg.in/yaml.v3"
)

// Embed the YAML file into the binary at build time
//go:embed ../../rules/presets.yaml
var embeddedRulesYAML []byte

// LoadEmbeddedRules loads the embedded rules and parses them
func LoadEmbeddedRules() (map[string]Preset, error) {
    var rulesFile RulesFile

    if err := yaml.Unmarshal(embeddedRulesYAML, &rulesFile); err != nil {
        return nil, fmt.Errorf("failed to parse embedded rules: %w", err)
    }

    if rulesFile.Version == "" {
        return nil, fmt.Errorf("rules file missing version")
    }

    return rulesFile.Presets, nil
}
```

### 3. Public API (`pkg/presets/registry.go`)

```go
package presets

import (
    "fmt"
    "sync"
)

var (
    // GlobalRegistry is loaded once at startup
    GlobalRegistry map[string]Preset
    once           sync.Once
    loadError      error
)

// Init loads the embedded rules (called at startup)
func Init() error {
    once.Do(func() {
        GlobalRegistry, loadError = LoadEmbeddedRules()
    })
    return loadError
}

// Get retrieves a preset by name
func Get(name string) (Preset, error) {
    if err := Init(); err != nil {
        return Preset{}, err
    }

    preset, exists := GlobalRegistry[name]
    if !exists {
        return Preset{}, fmt.Errorf("preset not found: %s", name)
    }
    return preset, nil
}

// List returns all preset names grouped by phase
func List() map[string][]string {
    if err := Init(); err != nil {
        return nil
    }

    phases := make(map[string][]string)
    for name, preset := range GlobalRegistry {
        phases[preset.Phase] = append(phases[preset.Phase], name)
    }
    return phases
}

// Exists checks if a preset exists
func Exists(name string) bool {
    if err := Init(); err != nil {
        return false
    }
    _, exists := GlobalRegistry[name]
    return exists
}

// GetByPhase returns all presets for a specific phase
func GetByPhase(phase string) []Preset {
    if err := Init(); err != nil {
        return nil
    }

    var presets []Preset
    for _, preset := range GlobalRegistry {
        if preset.Phase == phase {
            presets = append(presets, preset)
        }
    }
    return presets
}

// Count returns the total number of presets
func Count() int {
    if err := Init(); err != nil {
        return 0
    }
    return len(GlobalRegistry)
}
```

---

## Developer Workflow

### Adding a New Tool

**Example: Adding semgrep**

1. **Edit `rules/presets.yaml`:**

```yaml
presets:
  # ... existing tools

  semgrep:
    name: semgrep
    phase: security
    image: returntocorp/semgrep:1.52.0
    command: semgrep scan --config auto .
    workdir: /src
    volumes:
      - "${WORKSPACE}:/src"
    options:
      config:
        type: string
        default: auto
        flag: "--config"
        description: "Semgrep ruleset (auto, p/ci, p/security-audit)"
        values:
          - auto
          - p/ci
          - p/security-audit
          - p/owasp-top-ten
      json:
        type: bool
        default: false
        flag: "--json"
        description: "Output results in JSON format"
```

2. **Rebuild:**

```bash
make build
# or
go build -o bin/cidx ./cmd/cidx
```

3. **Test:**

```bash
./bin/cidx list
# Output should show:
#   security:
#     - gitleaks
#     - semgrep  ← New!
#     - trivy

./bin/cidx info semgrep
# Should display semgrep details

./bin/cidx run semgrep
# Should execute semgrep
```

**That's it! 3 simple steps.**

### Updating a Tool Version

**Example: Update trivy from 0.57.1 to 0.58.0**

1. **Edit `rules/presets.yaml`:**

```yaml
trivy:
  name: trivy
  image: aquasec/trivy:0.58.0 # ← Changed
```

2. **Rebuild:**

```bash
make build
```

3. **Release (optional):**

```bash
git add rules/presets.yaml
git commit -m "chore(presets): update trivy to 0.58.0"
git tag v0.2.0
git push --tags
```

Users install with:

```bash
go install github.com/arcker/cidx/cmd/cidx@latest
```

---

## Validation & CI

### Validate Rules Schema

**Script: `scripts/validate-rules.go`**

```go
package main

import (
    "fmt"
    "os"

    "github.com/arcker/cidx/pkg/presets"
    "gopkg.in/yaml.v3"
)

func main() {
    // Read rules file
    data, err := os.ReadFile("rules/presets.yaml")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error reading rules file: %v\n", err)
        os.Exit(1)
    }

    // Parse YAML
    var rulesFile presets.RulesFile
    if err := yaml.Unmarshal(data, &rulesFile); err != nil {
        fmt.Fprintf(os.Stderr, "Error parsing YAML: %v\n", err)
        os.Exit(1)
    }

    // Validate
    errors := 0

    // Check version
    if rulesFile.Version == "" {
        fmt.Println("❌ Missing version field")
        errors++
    }

    // Validate each preset
    for name, preset := range rulesFile.Presets {
        if preset.Name == "" {
            fmt.Printf("❌ Preset '%s': missing name\n", name)
            errors++
        }
        if preset.Phase == "" {
            fmt.Printf("❌ Preset '%s': missing phase\n", name)
            errors++
        }
        if preset.Image == "" {
            fmt.Printf("❌ Preset '%s': missing image\n", name)
            errors++
        }
        if preset.Command == "" {
            fmt.Printf("❌ Preset '%s': missing command\n", name)
            errors++
        }

        // Warn if using :latest
        if strings.HasSuffix(preset.Image, ":latest") {
            fmt.Printf("⚠️  Preset '%s': using :latest tag (prefer semantic version)\n", name)
        }
    }

    if errors > 0 {
        fmt.Printf("\n❌ Validation failed with %d errors\n", errors)
        os.Exit(1)
    }

    fmt.Printf("✓ Validation passed (%d presets)\n", len(rulesFile.Presets))
}
```

### GitHub Actions CI

```yaml
# .github/workflows/validate.yml
name: Validate Rules

on:
  pull_request:
    paths:
      - "rules/presets.yaml"

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.21"

      - name: Validate rules schema
        run: go run scripts/validate-rules.go

      - name: Check YAML syntax
        run: yamllint rules/presets.yaml

      - name: Test build
        run: go build -o bin/cidx ./cmd/cidx

      - name: Test presets loading
        run: |
          ./bin/cidx list
          ./bin/cidx info trivy
```

---

## Makefile Helpers

```makefile
# Makefile
.PHONY: build validate-rules test-presets

build:
	@echo "Building CIDX with embedded rules..."
	@go build -o bin/cidx ./cmd/cidx
	@echo "✓ Build complete ($(shell ./bin/cidx list | wc -l) presets loaded)"

validate-rules:
	@echo "Validating rules/presets.yaml..."
	@go run scripts/validate-rules.go
	@yamllint rules/presets.yaml
	@echo "✓ Rules are valid"

test-presets:
	@echo "Testing presets loading..."
	@go build -o bin/cidx ./cmd/cidx
	@./bin/cidx list
	@echo ""
	@./bin/cidx info trivy
	@echo "✓ Presets loaded successfully"

add-preset:
	@echo "Adding new preset interactively..."
	@go run scripts/add-preset.go

release:
	@echo "Creating release..."
	@./scripts/release.sh
```

---

## Migration from Current Code

### Step 1: Export Current Registry to YAML

**Script: `scripts/export-to-yaml.go`**

```go
package main

import (
    "fmt"
    "os"

    "github.com/arcker/cidx/pkg/presets"
    "gopkg.in/yaml.v3"
)

func main() {
    // Get current GlobalRegistry (hardcoded in registry.go)
    rules := presets.GlobalRegistry

    file := presets.RulesFile{
        Version: "1.0",
        Presets: rules,
    }

    data, err := yaml.Marshal(file)
    if err != nil {
        panic(err)
    }

    // Write to rules/presets.yaml
    os.MkdirAll("rules", 0755)
    if err := os.WriteFile("rules/presets.yaml", data, 0644); err != nil {
        panic(err)
    }

    fmt.Printf("✓ Exported %d presets to rules/presets.yaml\n", len(rules))
}
```

Run:

```bash
go run scripts/export-to-yaml.go
```

### Step 2: Refactor Presets Package

1. Create `pkg/presets/embedded.go` with `go:embed`
2. Modify `pkg/presets/registry.go` to use `LoadEmbeddedRules()`
3. Remove hardcoded presets from `registry.go`
4. Keep `types.go` and `MergeWith()` unchanged

### Step 3: Test

```bash
go build -o bin/cidx ./cmd/cidx
./bin/cidx list
# Should show same tools as before

./bin/cidx run trivy --dry-run
# Should work identically
```

**Migration time: ~1 hour**

---

## Benefits vs Current Approach

### Comparison Table

| Aspect           | Current (Go hardcoded) | New (YAML embedded)        |
| ---------------- | ---------------------- | -------------------------- |
| **Readability**  | Go structs (verbose)   | YAML (clean)               |
| **Editability**  | Requires Go knowledge  | Just edit YAML             |
| **Contribution** | Need PR + Go review    | Just edit one file         |
| **Build**        | Rebuild for any change | Rebuild for any change     |
| **Runtime**      | Parse Go structs       | Parse embedded YAML        |
| **Binary size**  | ~15MB                  | ~15MB (negligible diff)    |
| **Speed**        | Instant                | Instant (YAML parsed once) |
| **Deployment**   | Single binary          | Single binary              |

### Key Advantages

✅ **Easier to contribute** - No Go knowledge needed, just YAML  
✅ **Easier to review** - YAML diffs are clearer than Go diffs  
✅ **Easier to maintain** - One file vs scattered Go code  
✅ **Version control friendly** - Clean git diffs  
✅ **Documentation friendly** - YAML is self-documenting

### What Stays the Same

✅ **Zero-config** - Still works out-of-the-box  
✅ **Single binary** - No external files needed  
✅ **Fast** - YAML parsed once at startup  
✅ **Type-safe** - Go structs validate at build time

---

## Future Extensions (Phase 2, Optional)

### Local Overrides

**Support user-specific overrides** (enterprise use case):

```go
// pkg/presets/registry.go
func Init() error {
    once.Do(func() {
        // 1. Load embedded rules (baseline)
        GlobalRegistry, loadError = LoadEmbeddedRules()
        if loadError != nil {
            return
        }

        // 2. Load local overrides (optional)
        localPath := filepath.Join(os.Getenv("HOME"), ".config/cidx/rules.yaml")
        if fileExists(localPath) {
            localRules, err := loadYAMLFile(localPath)
            if err == nil {
                // Merge local overrides into global
                for name, preset := range localRules.Presets {
                    GlobalRegistry[name] = preset
                }
            }
        }
    })
    return loadError
}
```

**Usage:**

```yaml
# ~/.config/cidx/rules.yaml (optional, for custom overrides)
version: "1.0"
presets:
  trivy:
    # Override with internal registry
    image: registry.mybank.internal/trivy:0.57.1-approved
```

**But this is Phase 2. Keep it simple for now.**

---

## Summary: Why This Approach is KISS

### CIDX (Simple)

- 1 YAML file for all presets
- ~200 lines for 6 tools
- Embedded in binary at build
- Easy to understand and maintain

### The KISS Workflow

```
1. Edit rules/presets.yaml (one file)
2. go build (embed YAML)
3. Done (binary works standalone)
```

**No magic. No complexity. Just simple embedding.**

---

## Questions & Answers

### Q: Why not load YAML at runtime?

**A:** Requires YAML file to be distributed with binary. Embedding = single binary, zero-config.

### Q: What if YAML is invalid?

**A:** Build fails (YAML parsing error). Catches errors early.

### Q: Can users add their own presets?

**A:** Phase 2 feature (local overrides). Keep it simple for now.

### Q: What about preset versioning?

**A:** The `version` field in YAML. Can add compatibility checks later.

### Q: Performance impact?

**A:** Negligible. YAML parsed once at startup (~1ms). Presets cached in memory.

### Q: Binary size impact?

**A:** ~10KB for YAML file. Negligible compared to Go runtime (~10MB).

---

## Conclusion

This rules system achieves the perfect balance:

✅ **Simple** - One YAML file to edit  
✅ **Embedded** - No external files needed  
✅ **Standalone** - Binary works out-of-the-box  
✅ **Maintainable** - Easy to add/update tools  
✅ **Contributor-friendly** - No Go knowledge required

**It's KISS at its best.**
