package features

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/commands"
	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/cucumber/godog"
)

// RegisterCatalogueReferenceSteps registers the catalogue reference page steps
func RegisterCatalogueReferenceSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.When(`^the catalogue reference page is rendered$`, tc.catalogueReferenceIsRendered)
	ctx.When(`^the catalogue reference page is rendered twice$`, tc.catalogueReferenceIsRenderedTwice)
	ctx.Then(`^every built-in preset appears exactly once on the page$`, tc.everyPresetAppearsOnce)
	ctx.Then(`^the row for "([^"]*)" states image "([^"]*)", phase "([^"]*)" and capabilities "([^"]*)"$`, tc.rowStatesImagePhaseCapabilities)
	ctx.Then(`^the row for "([^"]*)" states update policy "([^"]*)"$`, tc.rowStatesUpdatePolicy)
	ctx.Then(`^both renderings are byte-identical$`, tc.renderingsAreIdentical)
}

func (tc *TestContext) catalogueReferenceIsRendered() error {
	page, err := commands.RenderCatalogueReference()
	if err != nil {
		return err
	}
	tc.Output = page
	return nil
}

func (tc *TestContext) catalogueReferenceIsRenderedTwice() error {
	first, err := commands.RenderCatalogueReference()
	if err != nil {
		return err
	}
	second, err := commands.RenderCatalogueReference()
	if err != nil {
		return err
	}
	tc.Output = first
	tc.Config["second_rendering"] = second
	return nil
}

func (tc *TestContext) everyPresetAppearsOnce() error {
	catalogue, err := presets.Catalogue()
	if err != nil {
		return err
	}
	for name := range catalogue {
		row := "| `" + name + "` |"
		if n := strings.Count(tc.Output, row); n != 1 {
			return fmt.Errorf("preset %q appears %d times on the page", name, n)
		}
	}
	return nil
}

// referenceRow finds one preset's table row.
func (tc *TestContext) referenceRow(preset string) (string, error) {
	re := regexp.MustCompile(`(?m)^\| ` + "`" + regexp.QuoteMeta(preset) + "`" + ` \|.*$`)
	row := re.FindString(tc.Output)
	if row == "" {
		return "", fmt.Errorf("no row for %q on the page", preset)
	}
	return row, nil
}

func (tc *TestContext) rowStatesImagePhaseCapabilities(preset, image, phase, capabilities string) error {
	row, err := tc.referenceRow(preset)
	if err != nil {
		return err
	}
	for _, want := range []string{"`" + image + "`", " " + phase + " ", capabilities} {
		if !strings.Contains(row, want) {
			return fmt.Errorf("row %q does not state %q", row, want)
		}
	}
	return nil
}

func (tc *TestContext) rowStatesUpdatePolicy(preset, policy string) error {
	row, err := tc.referenceRow(preset)
	if err != nil {
		return err
	}
	if !strings.Contains(row, "| "+policy+" |") {
		return fmt.Errorf("row %q does not state update policy %q", row, policy)
	}
	return nil
}

func (tc *TestContext) renderingsAreIdentical() error {
	second, _ := tc.Config["second_rendering"].(string)
	if tc.Output != second {
		return fmt.Errorf("the two renderings differ")
	}
	return nil
}
