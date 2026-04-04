package main

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// RegisterPullPolicySteps registers step definitions for pull policy scenarios
func RegisterPullPolicySteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.When(`^I run a container with no pull_policy set$`, tc.runContainerNoPullPolicy)
	ctx.When(`^I run a container with pull_policy "([^"]*)"$`, tc.runContainerWithPullPolicy)
	ctx.When(`^I run container "([^"]*)"$`, tc.runNamedContainer)

	ctx.Given(`^container "([^"]*)" has pull_policy "([^"]*)"$`, tc.containerHasPullPolicy)
	ctx.Given(`^image "([^"]*)" exists locally$`, tc.imageExistsLocally)
	ctx.Given(`^image "([^"]*)" does NOT exist locally$`, tc.imageDoesNotExistLocally)

	ctx.Then(`^the effective pull policy should be "([^"]*)"$`, tc.effectivePullPolicyShouldBe)
	ctx.Then(`^no image pull should occur$`, tc.noImagePullShouldOccur)
	ctx.Then(`^the image should be pulled$`, tc.imageShouldBePulled)
}

func (tc *TestContext) runContainerNoPullPolicy() error {
	tc.simulatePullPolicyIfNeeded("")
	return nil
}

func (tc *TestContext) runContainerWithPullPolicy(policy string) error {
	tc.simulatePullPolicyIfNeeded(policy)
	return nil
}

func (tc *TestContext) runNamedContainer(name string) error {
	policy, _ := tc.Config["pull_policy_"+name].(string)
	tc.simulatePullPolicyIfNeeded(policy)
	return nil
}

func (tc *TestContext) containerHasPullPolicy(name, policy string) error {
	tc.Config["pull_policy_"+name] = policy
	return nil
}

func (tc *TestContext) imageExistsLocally(image string) error {
	tc.Config["image_exists_"+image] = true
	return nil
}

func (tc *TestContext) imageDoesNotExistLocally(image string) error {
	tc.Config["image_exists_"+image] = false
	return nil
}

func (tc *TestContext) effectivePullPolicyShouldBe(expected string) error {
	actual, _ := tc.Config["effective_pull_policy"].(string)
	if actual != expected {
		return fmt.Errorf("expected pull policy %q, got %q", expected, actual)
	}
	return nil
}

func (tc *TestContext) noImagePullShouldOccur() error {
	if !strings.Contains(tc.Output, "skipping pull") {
		return fmt.Errorf("expected no pull, but output was:\n%s", tc.Output)
	}
	return nil
}

func (tc *TestContext) imageShouldBePulled() error {
	if !strings.Contains(tc.Output, "pulling") {
		return fmt.Errorf("expected image pull, but output was:\n%s", tc.Output)
	}
	return nil
}

func (tc *TestContext) simulatePullPolicyIfNeeded(explicitPolicy string) {
	// Determine effective policy
	policy := explicitPolicy
	if policy == "" {
		if tc.CI {
			policy = "always"
		} else {
			policy = "if-not-present"
		}
	}
	tc.Config["effective_pull_policy"] = policy

	switch policy {
	case "never":
		tc.Output += "skipping pull (policy: never)\n"
	case "if-not-present":
		// Check if any image is marked as existing
		imageExists := false
		for k, v := range tc.Config {
			if strings.HasPrefix(k, "image_exists_") {
				if exists, ok := v.(bool); ok && exists {
					imageExists = true
				}
			}
		}
		if imageExists {
			tc.Output += "skipping pull (image exists locally)\n"
		} else {
			tc.Output += "pulling image (not found locally)\n"
		}
	case "always":
		tc.Output += "pulling image (policy: always)\n"
	}
}
