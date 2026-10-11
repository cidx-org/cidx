package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/cucumber/godog"
)

// Steps for features/presets/alias.feature (#292). They load a real cidx.toml
// through config.Load and config.Validate, and resolve the container with
// presets.Lookup -- the call the runner makes -- so a scenario cannot pass on a
// resolution the runner does not perform.

type aliasWorld struct {
	toml     string
	problems []string // load or validation errors
	resolved presets.Preset
}

// RegisterPresetAliasSteps registers the preset alias step definitions.
func RegisterPresetAliasSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a cidx\.toml containing:$`, tc.cidxTomlContaining)
	ctx.When(`^the configuration is validated and "([^"]*)" is resolved$`, tc.validateAndResolve)
	ctx.Then(`^the configuration should be valid$`, tc.projectConfigurationShouldBeValid)
	ctx.Then(`^the configuration should be refused, saying "([^"]*)"$`, tc.configurationShouldBeRefused)
	ctx.Then(`^"([^"]*)" should run the image of the "([^"]*)" preset$`, tc.shouldRunImageOfPreset)
	ctx.Then(`^"([^"]*)" should run the image "([^"]*)"$`, tc.shouldRunImage)
	ctx.Then(`^"([^"]*)" should have a command containing "([^"]*)"$`, tc.aliasCommandShouldContain)
	ctx.Then(`^the preset "([^"]*)" should keep a command without "([^"]*)"$`, tc.presetCommandShouldLack)
	ctx.Then(`^"([^"]*)" should be resolved under the name "([^"]*)"$`, tc.shouldBeResolvedUnder)
}

func (tc *TestContext) alias() *aliasWorld {
	w, ok := tc.Config["alias"].(*aliasWorld)
	if !ok {
		w = &aliasWorld{}
		tc.Config["alias"] = w
	}
	return w
}

func (tc *TestContext) cidxTomlContaining(doc *godog.DocString) error {
	tc.alias().toml = doc.Content
	return nil
}

func (tc *TestContext) validateAndResolve(name string) error {
	w := tc.alias()
	w.problems = nil

	dir, err := os.MkdirTemp("", "cidx-alias-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "cidx.toml")
	if err := os.WriteFile(path, []byte(w.toml), 0o600); err != nil {
		return err
	}

	cfg, err := config.Load(path)
	if err != nil {
		w.problems = append(w.problems, err.Error())
		return nil
	}
	if result := config.Validate(cfg); !result.Valid {
		w.problems = append(w.problems, result.Errors...)
	}

	preset, overrides, err := presets.Lookup(name, cfg.Overrides[name])
	if err != nil {
		w.problems = append(w.problems, err.Error())
		return nil
	}
	w.resolved = *preset.MergeWith(overrides)
	return nil
}

func (tc *TestContext) projectConfigurationShouldBeValid() error {
	if w := tc.alias(); len(w.problems) > 0 {
		return fmt.Errorf("the configuration was refused: %s", strings.Join(w.problems, "; "))
	}
	return nil
}

func (tc *TestContext) configurationShouldBeRefused(fragment string) error {
	w := tc.alias()
	if len(w.problems) == 0 {
		return fmt.Errorf("the configuration was accepted, it should have been refused (%q)", fragment)
	}
	if all := strings.Join(w.problems, "; "); !strings.Contains(all, fragment) {
		return fmt.Errorf("refused, but not saying %q: %s", fragment, all)
	}
	return nil
}

func (tc *TestContext) shouldRunImageOfPreset(_, presetName string) error {
	base, err := presets.Get(presetName)
	if err != nil {
		return err
	}
	return tc.shouldRunImage("", base.Image)
}

func (tc *TestContext) shouldRunImage(_, image string) error {
	if got := tc.alias().resolved.Image; got != image {
		return fmt.Errorf("resolved image = %q, want %q", got, image)
	}
	return nil
}

func (tc *TestContext) aliasCommandShouldContain(_, fragment string) error {
	if got := tc.alias().resolved.Command; !strings.Contains(got, fragment) {
		return fmt.Errorf("resolved command %q does not contain %q", got, fragment)
	}
	return nil
}

func (tc *TestContext) presetCommandShouldLack(presetName, fragment string) error {
	base, err := presets.Get(presetName)
	if err != nil {
		return err
	}
	if strings.Contains(base.Command, fragment) {
		return fmt.Errorf("the catalogue preset %q was changed by its alias: command %q", presetName, base.Command)
	}
	return nil
}

func (tc *TestContext) shouldBeResolvedUnder(_, name string) error {
	if got := tc.alias().resolved.Name; got != name {
		return fmt.Errorf("resolved name = %q, want %q", got, name)
	}
	return nil
}
