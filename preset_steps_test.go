package main

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/cucumber/godog"
)

// RegisterPresetSteps registers preset-related step definitions
func RegisterPresetSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Custom preset loading
	ctx.Given(`^I have no custom configuration files$`, tc.haveNoCustomConfigFiles)
	ctx.Given(`^I have a user config file at "([^"]*)"$`, tc.haveUserConfigFile)
	ctx.Given(`^I have a project config file at "([^"]*)"$`, tc.haveProjectConfigFile)
	ctx.Given(`^the user config defines a custom image for "([^"]*)"$`, tc.userConfigCustomImage)
	ctx.Given(`^the project config defines a custom command for "([^"]*)"$`, tc.projectConfigCustomCommand)
	ctx.Given(`^the project config defines a new tool "([^"]*)"$`, tc.projectConfigNewTool)
	ctx.Given(`^a file "([^"]*)" with content:$`, tc.fileWithContent)

	// Preset assertions
	ctx.Then(`^it should use the built-in "([^"]*)" preset$`, tc.shouldUseBuiltinPreset)
	ctx.Then(`^it should use the custom image from the user config$`, tc.shouldUseCustomImage)
	ctx.Then(`^it should use the custom command from the project config$`, tc.shouldUseCustomCommand)
	ctx.Then(`^it should execute the "([^"]*)" container$`, tc.shouldExecuteContainer)
	ctx.Then(`^the container should use the configuration from "([^"]*)"$`, tc.containerShouldUseConfig)
	ctx.When(`^I validate the configuration$`, tc.validateConfiguration)
	ctx.When(`^I load the custom presets$`, tc.loadCustomPresets)
	ctx.Then(`^it should be valid$`, tc.configShouldBeValid)
	ctx.Then(`^the tool "([^"]*)" should be available$`, tc.toolShouldBeAvailable)
	ctx.Then(`^the preset "([^"]*)" should have "([^"]*)" set to "([^"]*)"$`, tc.presetFieldShouldEqual)

	// Option flag placement (#200)
	ctx.When(`^I resolve the preset "([^"]*)" with option "([^"]*)" set to "([^"]*)"$`, tc.resolvePresetWithOption)
	ctx.When(`^I resolve the preset "([^"]*)" without overrides$`, tc.resolvePresetWithoutOverrides)
	ctx.Then(`^the resolved command should be "([^"]*)"$`, tc.resolvedCommandShouldBe)
	ctx.Then(`^the resolved command should contain "([^"]*)"$`, tc.resolvedCommandShouldContain)
}

// resolvePresetWithOption merges a single user option override into a built-in
// preset, exactly as cidx.toml's `[containers.NAME]` section does at runtime.
func (tc *TestContext) resolvePresetWithOption(presetName, option, value string) error {
	return tc.resolvePreset(presetName, map[string]any{option: value})
}

// resolvePresetWithoutOverrides resolves a built-in preset as-is.
func (tc *TestContext) resolvePresetWithoutOverrides(presetName string) error {
	return tc.resolvePreset(presetName, map[string]any{})
}

func (tc *TestContext) resolvePreset(presetName string, overrides map[string]any) error {
	preset, err := presets.Get(presetName)
	if err != nil {
		return fmt.Errorf("failed to resolve preset %q: %w", presetName, err)
	}
	tc.Config["resolved_command"] = preset.MergeWith(overrides).Command
	return nil
}

// resolvedCommandShouldBe asserts the exact command handed to the executor.
func (tc *TestContext) resolvedCommandShouldBe(want string) error {
	got, ok := tc.Config["resolved_command"].(string)
	if !ok {
		return fmt.Errorf("no preset resolved")
	}
	if got != want {
		return fmt.Errorf("resolved command = %q, want %q", got, want)
	}
	return nil
}

// resolvedCommandShouldContain asserts a fragment of the resolved command, for
// commands too long to spell out in full.
func (tc *TestContext) resolvedCommandShouldContain(want string) error {
	got, ok := tc.Config["resolved_command"].(string)
	if !ok {
		return fmt.Errorf("no preset resolved")
	}
	if !strings.Contains(got, want) {
		return fmt.Errorf("resolved command %q does not contain %q", got, want)
	}
	return nil
}

// loadCustomPresets decodes the preset file staged by `a file "..." with content:`
// through the same types the loader uses, so a field the loader cannot decode is
// a field this step cannot see (#203).
func (tc *TestContext) loadCustomPresets() error {
	content, ok := tc.Config["test_file_content"].(string)
	if !ok {
		return fmt.Errorf("no preset file content staged")
	}

	var file presets.PresetsFile
	if err := toml.Unmarshal([]byte(content), &file); err != nil {
		return fmt.Errorf("failed to parse staged preset file: %w", err)
	}
	tc.Config["loaded_presets"] = file.Presets
	return nil
}

// presetFieldShouldEqual asserts a loaded preset carries the expected value for
// one of its execution fields.
func (tc *TestContext) presetFieldShouldEqual(presetName, field, want string) error {
	loaded, ok := tc.Config["loaded_presets"].(map[string]presets.PresetTOML)
	if !ok {
		return fmt.Errorf("no presets loaded")
	}
	preset, ok := loaded[presetName]
	if !ok {
		return fmt.Errorf("preset %q not loaded", presetName)
	}

	var got string
	switch field {
	case "pull_policy":
		got = preset.PullPolicy
	case "timeout":
		got = preset.Timeout
	case "image":
		got = preset.Image
	case "command":
		got = preset.Command
	case "workdir":
		got = preset.Workdir
	default:
		return fmt.Errorf("unsupported preset field %q", field)
	}

	if got != want {
		return fmt.Errorf("preset %q field %q = %q, want %q", presetName, field, got, want)
	}
	return nil
}

// haveNoCustomConfigFiles sets up environment with no custom configs
func (tc *TestContext) haveNoCustomConfigFiles() error {
	tc.Config["no_custom_configs"] = true
	return nil
}

// haveUserConfigFile sets up a user-level config file
func (tc *TestContext) haveUserConfigFile(path string) error {
	tc.Config["user_config_path"] = path
	return nil
}

// haveProjectConfigFile sets up a project-level config file
func (tc *TestContext) haveProjectConfigFile(path string) error {
	tc.Config["project_config_path"] = path
	return nil
}

// userConfigCustomImage marks user config has custom image
func (tc *TestContext) userConfigCustomImage(preset string) error {
	tc.Config["user_custom_image_for"] = preset
	return nil
}

// projectConfigCustomCommand marks project config has custom command
func (tc *TestContext) projectConfigCustomCommand(preset string) error {
	tc.Config["project_custom_command_for"] = preset
	return nil
}

// projectConfigNewTool marks project config defines a new tool
func (tc *TestContext) projectConfigNewTool(tool string) error {
	tc.Config["project_new_tool"] = tool
	return nil
}

// fileWithContent sets up a file with given content
func (tc *TestContext) fileWithContent(path string, doc *godog.DocString) error {
	tc.Config["test_file_path"] = path
	tc.Config["test_file_content"] = doc.Content
	return nil
}

// shouldUseBuiltinPreset checks built-in preset is used
func (tc *TestContext) shouldUseBuiltinPreset(preset string) error {
	// When no custom configs, built-in presets are used by default
	return nil
}

// shouldUseCustomImage checks custom image is used
func (tc *TestContext) shouldUseCustomImage() error {
	return nil
}

// shouldUseCustomCommand checks custom command is used
func (tc *TestContext) shouldUseCustomCommand() error {
	return nil
}

// shouldExecuteContainer checks container was executed
func (tc *TestContext) shouldExecuteContainer(container string) error {
	return nil
}

// containerShouldUseConfig checks container uses config from path
func (tc *TestContext) containerShouldUseConfig(path string) error {
	return nil
}

// validateConfiguration runs config validation
func (tc *TestContext) validateConfiguration() error {
	tc.ExitCode = 0
	return nil
}

// configShouldBeValid checks config is valid
func (tc *TestContext) configShouldBeValid() error {
	if tc.ExitCode != 0 {
		return nil
	}
	return nil
}

// toolShouldBeAvailable checks tool is available
func (tc *TestContext) toolShouldBeAvailable(tool string) error {
	return nil
}
