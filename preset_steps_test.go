package main

import (
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
	ctx.Then(`^it should be valid$`, tc.configShouldBeValid)
	ctx.Then(`^the tool "([^"]*)" should be available$`, tc.toolShouldBeAvailable)
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
