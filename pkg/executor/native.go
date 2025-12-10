package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/sirupsen/logrus"
)

// NativeCommand defines how to run a tool natively (without container)
type NativeCommand struct {
	Binary      string   // Binary name to execute
	Args        []string // Default arguments
	InstallHint string   // How to install this tool
}

// nativeCommands maps preset names to their native execution config
var nativeCommands = map[string]NativeCommand{
	// Go tools
	"golangci-lint": {
		Binary:      "golangci-lint",
		Args:        []string{"run"},
		InstallHint: "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
	},
	"go-test": {
		Binary:      "go",
		Args:        []string{"test", "./..."},
		InstallHint: "Go is required: https://go.dev/dl/",
	},
	"go-build": {
		Binary:      "go",
		Args:        []string{"build", "./..."},
		InstallHint: "Go is required: https://go.dev/dl/",
	},
	"gofmt": {
		Binary:      "gofmt",
		Args:        []string{"-l", "."},
		InstallHint: "Go is required: https://go.dev/dl/",
	},
	"go-mod-tidy": {
		Binary:      "go",
		Args:        []string{"mod", "tidy"},
		InstallHint: "Go is required: https://go.dev/dl/",
	},

	// JavaScript/Node tools
	"prettier": {
		Binary:      "npx",
		Args:        []string{"prettier", "--check", "."},
		InstallHint: "npm install -g prettier",
	},
	"eslint": {
		Binary:      "npx",
		Args:        []string{"eslint", "."},
		InstallHint: "npm install -g eslint",
	},

	// Security tools
	"trivy": {
		Binary:      "trivy",
		Args:        []string{"fs", "."},
		InstallHint: "brew install trivy (macOS) or apt install trivy (Linux)",
	},
	"gitleaks": {
		Binary:      "gitleaks",
		Args:        []string{"git", "."},
		InstallHint: "brew install gitleaks (macOS) or go install github.com/gitleaks/gitleaks/v8@latest",
	},
	"grype": {
		Binary:      "grype",
		Args:        []string{"dir:."},
		InstallHint: "brew install grype (macOS) or curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s",
	},

	// Python tools
	"ruff": {
		Binary:      "ruff",
		Args:        []string{"check", "."},
		InstallHint: "pip install ruff",
	},
	"black": {
		Binary:      "black",
		Args:        []string{"--check", "."},
		InstallHint: "pip install black",
	},
	"mypy": {
		Binary:      "mypy",
		Args:        []string{"."},
		InstallHint: "pip install mypy",
	},

	// Rust tools
	"cargo-clippy": {
		Binary:      "cargo",
		Args:        []string{"clippy"},
		InstallHint: "rustup component add clippy",
	},
	"cargo-fmt": {
		Binary:      "cargo",
		Args:        []string{"fmt", "--check"},
		InstallHint: "rustup component add rustfmt",
	},
}

// NativeExecutor executes tools directly on the host system
type NativeExecutor struct {
	logger  *logrus.Logger
	dryRun  bool
	verbose bool
}

// NewNativeExecutor creates a new native executor
func NewNativeExecutor(dryRun, verbose bool) *NativeExecutor {
	logger := logrus.New()
	if verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	return &NativeExecutor{
		logger:  logger,
		dryRun:  dryRun,
		verbose: verbose,
	}
}

// Run executes a tool natively
func (e *NativeExecutor) Run(ctx context.Context, containerConfig *config.ContainerConfig) error {
	toolName := containerConfig.Name

	nativeCmd, exists := nativeCommands[toolName]
	if !exists {
		return fmt.Errorf("tool '%s' has no native execution support", toolName)
	}

	// Check if binary is available
	binaryPath, err := exec.LookPath(nativeCmd.Binary)
	if err != nil {
		return fmt.Errorf("tool '%s' not found locally. Install with: %s", toolName, nativeCmd.InstallHint)
	}

	e.logger.Infof("  ▸ Running [%s] %s (native)", containerConfig.Phase, toolName)

	// Build command with default args
	args := make([]string, len(nativeCmd.Args))
	copy(args, nativeCmd.Args)

	if e.dryRun {
		e.printDryRun(toolName, binaryPath, args, containerConfig.Env)
		return nil
	}

	// Create command
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = "." // Use current directory as workspace
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range containerConfig.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, os.ExpandEnv(v)))
	}

	e.logger.Debugf("Executing: %s %s", binaryPath, strings.Join(args, " "))

	// Run command
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("tool exited with code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to execute: %w", err)
	}

	e.logger.Infof("  ✓ %s completed (native)", toolName)
	return nil
}

// printDryRun shows what would be executed
func (e *NativeExecutor) printDryRun(toolName, binary string, args []string, env map[string]string) {
	fmt.Printf("Would execute (native):\n")
	fmt.Printf("  Tool: %s\n", toolName)
	fmt.Printf("  Binary: %s\n", binary)
	fmt.Printf("  Command: %s %s\n", binary, strings.Join(args, " "))
	fmt.Printf("  Workdir: .\n")
	if len(env) > 0 {
		fmt.Printf("  Environment:\n")
		for k, v := range env {
			fmt.Printf("    %s=%s\n", k, v)
		}
	}
	fmt.Println()
}

// Available always returns true for native executor
// Individual tool availability is checked via CanRun
func (e *NativeExecutor) Available() bool {
	return true
}

// Name returns the executor backend name
func (e *NativeExecutor) Name() string {
	return "native"
}

// Close is a no-op for native executor
func (e *NativeExecutor) Close() error {
	return nil
}

// CanRun checks if a specific tool can be run natively
func (e *NativeExecutor) CanRun(toolName string) bool {
	nativeCmd, exists := nativeCommands[toolName]
	if !exists {
		return false
	}

	_, err := exec.LookPath(nativeCmd.Binary)
	return err == nil
}

// GetInstallHint returns installation instructions for a tool
func (e *NativeExecutor) GetInstallHint(toolName string) string {
	if nativeCmd, exists := nativeCommands[toolName]; exists {
		return nativeCmd.InstallHint
	}
	return fmt.Sprintf("No native support for '%s'. Use Docker instead.", toolName)
}

// GetSupportedTools returns list of tools with native support
func GetSupportedTools() []string {
	tools := make([]string, 0, len(nativeCommands))
	for name := range nativeCommands {
		tools = append(tools, name)
	}
	return tools
}

// Ensure NativeExecutor implements Executor and NativeCapable interfaces
var _ Executor = (*NativeExecutor)(nil)
var _ NativeCapable = (*NativeExecutor)(nil)
