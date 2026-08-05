package actions

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// refExistsOnRemote reports whether ref is a branch or tag on origin. It is a
// package variable so tests replace it without a repository or a network call,
// the same seam convention the CI-timeout and registry code use.
var refExistsOnRemote = func(ref string) (bool, error) {
	out, err := vcs.Git("", "ls-remote", "--heads", "--tags", "origin", ref).Output()
	if err != nil {
		return false, fmt.Errorf("git ls-remote failed: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// WorkflowRunAction triggers a workflow on a ref and, by default, watches the
// run it started. It closes the last hole in the dogfooding loop: changing a
// workflow used to mean dropping to `gh workflow run` to try it (issue #266).
type WorkflowRunAction struct {
	provider remote.Provider
	workflow string
	ref      string
	inputs   map[string]string
	watch    bool
}

// NewWorkflowRun creates a workflow run action. ref must already be resolved by
// the caller (the CLI defaults it to the current branch).
func NewWorkflowRun(provider remote.Provider, workflow, ref string, inputs map[string]string, watch bool) *WorkflowRunAction {
	return &WorkflowRunAction{
		provider: provider,
		workflow: workflow,
		ref:      ref,
		inputs:   inputs,
		watch:    watch,
	}
}

// ParseWorkflowInputs turns repeated `key=value` flags into the input map the
// provider takes. Values are passed through unchanged, including empty ones;
// only a malformed pair or a repeated key is rejected, because both mean the
// user asked for something other than what would be sent.
func ParseWorkflowInputs(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	inputs := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("invalid input %q: expected key=value", pair)
		}
		if _, dup := inputs[key]; dup {
			return nil, fmt.Errorf("input %q given twice", key)
		}
		inputs[key] = value
	}

	return inputs, nil
}

// Execute triggers the workflow and, unless watching was turned off, follows
// the run it created until it completes.
func (a *WorkflowRunAction) Execute(ctx context.Context) error {
	if a.workflow == "" {
		return fmt.Errorf("a workflow name is required")
	}
	if a.ref == "" {
		return fmt.Errorf("a ref is required")
	}

	if err := a.checkRefIsPushed(); err != nil {
		return err
	}

	log.Infof("🚀 Triggering workflow '%s' on '%s'...", a.workflow, a.ref)
	for _, key := range sortedKeys(a.inputs) {
		log.Infof("   input %s=%s", key, a.inputs[key])
	}

	run, err := a.provider.TriggerWorkflow(ctx, a.workflow, a.ref, a.inputs)
	if err != nil {
		return err
	}

	log.Infof("✅ Run %s started", run.ID)
	log.Infof("🔗 %s", run.URL)

	if !a.watch {
		log.Infof("💡 Watch it with: cidx workflow watch %s", run.ID)
		return nil
	}

	return NewWorkflowWatch(a.provider, "", "", run.ID, false).Execute(ctx)
}

// checkRefIsPushed fails early on a ref the remote does not have. The dispatch
// would otherwise fail with GitHub's "No ref found for: x", which does not say
// that the fix is to push the branch (issue #266). A check that cannot run --
// no origin, no network -- warns and lets the dispatch decide.
func (a *WorkflowRunAction) checkRefIsPushed() error {
	exists, err := refExistsOnRemote(a.ref)
	if err != nil {
		log.Warnf("⚠️  Could not verify that '%s' exists on origin: %v", a.ref, err)
		return nil
	}
	if !exists {
		return fmt.Errorf("ref '%s' is not on origin -- a workflow can only run on a pushed ref. Push it first: cidx cpw -m \"...\"", a.ref)
	}
	return nil
}

// sortedKeys keeps the echoed inputs in a stable order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
