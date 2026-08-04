package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/internal/commands"
	"github.com/cidx-org/cidx/v2/pkg/actions"
	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cucumber/godog"
	"github.com/urfave/cli/v2"
)

// RegisterArtifactWorkflowSteps registers the steps for downloading a run's
// artifacts (issue #285) and restarting a run (issue #342).
//
// The behaviour steps drive the real actions -- actions.ArtifactDownloadAction
// and actions.WorkflowRerunAction, the ones the commands construct -- against a
// provider that answers from the scenario's table. Nothing here opens a socket:
// the provider boundary is where the network stops, and these scenarios sit on
// the near side of it.
//
// The command-line steps resolve against commands.NewApp(), the tree the binary
// runs (issue #317), so a renamed flag or a moved default fails here.
func RegisterArtifactWorkflowSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^run "([^"]*)" produced artifacts:$`, tc.runProducedArtifacts)
	ctx.Given(`^run "([^"]*)" ended with jobs:$`, tc.runEndedWithJobs)
	ctx.Given(`^run "([^"]*)" is unknown to the provider$`, tc.runIsUnknown)

	ctx.When(`^I download the artifacts of run "([^"]*)"$`, func(run string) error {
		return tc.downloadArtifacts(run, "")
	})
	ctx.When(`^I download the artifacts of run "([^"]*)" matching "([^"]*)"$`, tc.downloadArtifacts)
	ctx.When(`^I rerun the failed jobs of run "([^"]*)"$`, func(run string) error {
		return tc.rerunRun(run, true)
	})
	ctx.When(`^I rerun run "([^"]*)"$`, func(run string) error {
		return tc.rerunRun(run, false)
	})
	ctx.When(`^I list the runs of branch "([^"]*)"$`, func(branch string) error {
		return tc.listRuns("", branch)
	})
	ctx.When(`^I list the runs of workflow "([^"]*)"$`, func(workflow string) error {
		return tc.listRuns(workflow, "")
	})

	ctx.Then(`^the results directory holds exactly:$`, tc.resultsDirectoryHoldsExactly)
	ctx.Then(`^"([^"]*)" holds "([^"]*)"$`, tc.resultFileHolds)
	ctx.Then(`^the download is refused$`, tc.theRequestIsRefused)
	ctx.Then(`^the rerun is refused$`, tc.theRequestIsRefused)
	ctx.Then(`^the refusal names "([^"]*)"$`, tc.theRefusalNames)
	ctx.Then(`^the failed jobs of run "([^"]*)" are restarted$`, func(run string) error {
		return tc.rerunWasRequested(run, true)
	})
	ctx.Then(`^every job of run "([^"]*)" are restarted$`, func(run string) error {
		return tc.rerunWasRequested(run, false)
	})
	ctx.Then(`^every job of run "([^"]*)" is restarted$`, func(run string) error {
		return tc.rerunWasRequested(run, false)
	})
	ctx.Then(`^no rerun is requested from the provider$`, tc.noRerunWasRequested)
	ctx.Then(`^the provider is asked for every workflow on "([^"]*)"$`, func(branch string) error {
		return tc.providerWasAskedFor("", branch)
	})
	ctx.Then(`^the provider is asked for workflow "([^"]*)" on every branch$`, func(workflow string) error {
		return tc.providerWasAskedFor(workflow, "")
	})
	ctx.Then(`^the listing names the workflow of each run$`, tc.listingNamesEachWorkflow)
	ctx.Then(`^the default of "([^"]*)" is the default of "([^"]*)"$`, tc.defaultsAgree)
}

// stubProvider answers the four calls these two commands make, from what the
// scenario staged. Every other method of remote.Provider panics: a scenario that
// reaches one is asking a question these steps do not model, and silence would
// hide that.
type stubProvider struct {
	artifacts map[string][]remote.Artifact
	archives  map[int64][]byte
	runs      map[string]*remote.Workflow
	listed    []remote.Workflow

	rerunCalls []string
	listCalls  []string
}

func (s *stubProvider) ListRunArtifacts(_ context.Context, runID string) ([]remote.Artifact, error) {
	return s.artifacts[runID], nil
}

func (s *stubProvider) DownloadArtifact(_ context.Context, artifactID int64) (io.ReadCloser, error) {
	archive, ok := s.archives[artifactID]
	if !ok {
		return nil, fmt.Errorf("no artifact %d", artifactID)
	}
	return io.NopCloser(bytes.NewReader(archive)), nil
}

func (s *stubProvider) GetWorkflowRun(_ context.Context, runID string) (*remote.Workflow, error) {
	run, ok := s.runs[runID]
	if !ok {
		return nil, errors.New("404 Not Found")
	}
	return run, nil
}

func (s *stubProvider) RerunWorkflow(_ context.Context, runID string, failedOnly bool) error {
	s.rerunCalls = append(s.rerunCalls, fmt.Sprintf("%s failed=%t", runID, failedOnly))
	return nil
}

func (s *stubProvider) ListRuns(_ context.Context, workflowFile, branch string, _ int) ([]remote.Workflow, error) {
	s.listCalls = append(s.listCalls, fmt.Sprintf("workflow=%q branch=%q", workflowFile, branch))
	return s.listed, nil
}

func (s *stubProvider) GetLatestWorkflow(context.Context, string) (*remote.Workflow, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) GetLatestRunForBranch(context.Context, string) (*remote.Workflow, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) GetLatestRunForTag(context.Context, string) (*remote.Workflow, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) WatchWorkflow(context.Context, string) (<-chan remote.WorkflowUpdate, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) TriggerWorkflow(context.Context, string, string, map[string]string) (*remote.Workflow, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) CreatePullRequest(context.Context, string, string, string, string, bool) (int, string, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) MarkPullRequestReady(context.Context, int) error {
	panic("not reached by these scenarios")
}
func (s *stubProvider) GetPullRequestByBranch(context.Context, string) (int, string, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) MergePullRequest(context.Context, int, string) error {
	panic("not reached by these scenarios")
}
func (s *stubProvider) UpdatePullRequest(context.Context, int, string, string) error {
	panic("not reached by these scenarios")
}
func (s *stubProvider) GetPullRequestChecks(context.Context, int) (*remote.PRChecks, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) WaitForChecksToStart(context.Context, int, string, time.Duration) (string, *remote.PRChecks, error) {
	panic("not reached by these scenarios")
}
func (s *stubProvider) WatchPullRequestChecks(context.Context, int) (<-chan remote.PRChecksUpdate, error) {
	panic("not reached by these scenarios")
}

var _ remote.Provider = (*stubProvider)(nil)

// provider returns the scenario's provider, creating it on first use.
func (tc *TestContext) provider() *stubProvider {
	if existing, ok := tc.Config["provider"].(*stubProvider); ok {
		return existing
	}
	stub := &stubProvider{
		artifacts: make(map[string][]remote.Artifact),
		archives:  make(map[int64][]byte),
		runs:      make(map[string]*remote.Workflow),
	}
	tc.Config["provider"] = stub
	return stub
}

// resultsDir is the destination for the scenario's download, created once.
func (tc *TestContext) resultsDir() string {
	if dir, ok := tc.Config["resultsDir"].(string); ok {
		return dir
	}
	dir, err := os.MkdirTemp("", "cidx-artifacts-")
	if err != nil {
		panic(err)
	}
	tc.Config["resultsDir"] = dir
	// The suite's Cleanup removes GitRepo; reusing it keeps the temp directory
	// from outliving the scenario.
	tc.GitRepo = dir
	return dir
}

// runProducedArtifacts stages a run's artifacts, one zip per artifact name, with
// the rows of the table as its entries.
func (tc *TestContext) runProducedArtifacts(runID string, table *godog.Table) error {
	stub := tc.provider()

	byArtifact := make(map[string]map[string]string)
	var order []string

	for _, row := range table.Rows[1:] {
		name, file, content := row.Cells[0].Value, row.Cells[1].Value, row.Cells[2].Value
		if _, seen := byArtifact[name]; !seen {
			byArtifact[name] = make(map[string]string)
			order = append(order, name)
		}
		byArtifact[name][file] = content
	}

	for index, name := range order {
		id := int64(index + 1)
		archive, err := zipArchive(byArtifact[name])
		if err != nil {
			return err
		}
		stub.archives[id] = archive
		stub.artifacts[runID] = append(stub.artifacts[runID], remote.Artifact{ID: id, Name: name})
	}
	return nil
}

// zipArchive builds one artifact's archive, entries in a stable order so the
// "first copy wins" rule is asserted on something deterministic.
func zipArchive(entries map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		file, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write([]byte(entries[name])); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// runEndedWithJobs stages a completed run and the conclusion of each of its jobs.
func (tc *TestContext) runEndedWithJobs(runID string, table *godog.Table) error {
	run := &remote.Workflow{
		ID:     runID,
		Status: "completed",
		URL:    "https://github.com/cidx-org/cidx/actions/runs/" + runID,
	}

	run.Conclusion = "success"
	for _, row := range table.Rows[1:] {
		conclusion := row.Cells[1].Value
		run.Jobs = append(run.Jobs, remote.Job{
			Name: row.Cells[0].Value, Status: "completed", Conclusion: conclusion,
		})
		if conclusion != "success" {
			run.Conclusion = conclusion
		}
	}

	tc.provider().runs[runID] = run
	return nil
}

// runIsUnknown leaves the run out of the provider, which is what a run number
// handed over in place of a run ID looks like.
func (tc *TestContext) runIsUnknown(runID string) error {
	delete(tc.provider().runs, runID)
	return nil
}

func (tc *TestContext) downloadArtifacts(runID, pattern string) error {
	var patterns []string
	if pattern != "" {
		patterns = []string{pattern}
	}

	action := actions.NewArtifactDownload(tc.provider(), runID, patterns, tc.resultsDir())
	tc.record(action.Execute(context.Background()))
	return nil
}

func (tc *TestContext) rerunRun(runID string, failedOnly bool) error {
	action := actions.NewWorkflowRerun(tc.provider(), runID, failedOnly)
	tc.record(action.Execute(context.Background()))
	return nil
}

func (tc *TestContext) listRuns(workflow, branch string) error {
	stub := tc.provider()
	stub.listed = []remote.Workflow{
		{ID: "18234567890", Number: 640, Name: "CI", Branch: "main", Status: "completed", Conclusion: "failure"},
		{ID: "18234500000", Number: 639, Name: "Security Audit", Branch: "main", Status: "completed", Conclusion: "success"},
	}

	action := actions.NewWorkflowList(stub, workflow, branch, 10, false)
	tc.Output, tc.ExitCode = "", 0
	out, err := captureOutput(func() error { return action.Execute(context.Background()) })
	tc.Output = out
	if err != nil {
		tc.Output, tc.ExitCode = err.Error(), 1
	}
	return nil
}

// record stores the outcome of an action the way the CLI would report it.
func (tc *TestContext) record(err error) {
	tc.Output, tc.ExitCode = "", 0
	if err != nil {
		tc.Output, tc.ExitCode = err.Error(), 1
	}
}

// captureOutput collects what a listing prints, which is where its content is.
func captureOutput(fn func() error) (string, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return "", err
	}
	original := os.Stdout
	os.Stdout = write

	done := make(chan string)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, readErr := read.Read(buf)
			sb.Write(buf[:n])
			if readErr != nil {
				break
			}
		}
		done <- sb.String()
	}()

	runErr := fn()
	_ = write.Close()
	os.Stdout = original
	return <-done, runErr
}

func (tc *TestContext) resultsDirectoryHoldsExactly(table *godog.Table) error {
	if tc.ExitCode != 0 {
		return fmt.Errorf("the download failed: %s", tc.Output)
	}

	var want []string
	for _, row := range table.Rows {
		want = append(want, row.Cells[0].Value)
	}
	sort.Strings(want)

	entries, err := os.ReadDir(tc.resultsDir())
	if err != nil {
		return err
	}
	var got []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		got = append(got, name)
	}
	sort.Strings(got)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		return fmt.Errorf("want %v in the results directory, got %v", want, got)
	}
	return nil
}

func (tc *TestContext) resultFileHolds(name, content string) error {
	data, err := os.ReadFile(filepath.Join(tc.resultsDir(), name))
	if err != nil {
		return err
	}
	if string(data) != content {
		return fmt.Errorf("want %s to hold %q, got %q", name, content, data)
	}
	return nil
}

func (tc *TestContext) theRequestIsRefused() error {
	if tc.ExitCode == 0 {
		return errors.New("the request succeeded")
	}
	return nil
}

func (tc *TestContext) theRefusalNames(text string) error {
	if !strings.Contains(tc.Output, text) {
		return fmt.Errorf("expected the refusal to name %q, got:\n%s", text, tc.Output)
	}
	return nil
}

func (tc *TestContext) rerunWasRequested(runID string, failedOnly bool) error {
	want := fmt.Sprintf("%s failed=%t", runID, failedOnly)
	calls := tc.provider().rerunCalls
	for _, call := range calls {
		if call == want {
			return nil
		}
	}
	return fmt.Errorf("expected the provider to be asked for %q, got %v", want, calls)
}

func (tc *TestContext) noRerunWasRequested() error {
	if calls := tc.provider().rerunCalls; len(calls) > 0 {
		return fmt.Errorf("the provider was asked to rerun anyway: %v", calls)
	}
	return nil
}

func (tc *TestContext) providerWasAskedFor(workflow, branch string) error {
	want := fmt.Sprintf("workflow=%q branch=%q", workflow, branch)
	calls := tc.provider().listCalls
	for _, call := range calls {
		if call == want {
			return nil
		}
	}
	return fmt.Errorf("expected the listing to ask for %s, got %v", want, calls)
}

func (tc *TestContext) listingNamesEachWorkflow() error {
	for _, name := range []string{"CI", "Security Audit"} {
		if !strings.Contains(tc.Output, name) {
			return fmt.Errorf("the listing does not name the workflow %q:\n%s", name, tc.Output)
		}
	}
	return nil
}

// defaultsAgree pins the contract between the command that writes the results
// and the commands that read them: one directory, spelled once. Both defaults
// are read off the tree cidx ships, so moving either alone fails here.
func (tc *TestContext) defaultsAgree(writer, reader string) error {
	writerValue, err := flagDefault(writer)
	if err != nil {
		return err
	}
	readerValue, err := flagDefault(reader)
	if err != nil {
		return err
	}
	if writerValue != readerValue {
		return fmt.Errorf("%s defaults to %q while %s defaults to %q: the flow needs the path spelled twice",
			writer, writerValue, reader, readerValue)
	}
	return nil
}

// flagDefault resolves "cidx a b --flag" against the real tree and returns the
// flag's default value.
func flagDefault(line string) (string, error) {
	tokens, err := splitCommandLine(line)
	if err != nil {
		return "", err
	}
	if len(tokens) < 2 || tokens[0] != "cidx" {
		return "", fmt.Errorf("expected a command line ending in a flag, got %q", line)
	}

	flagName := strings.TrimPrefix(tokens[len(tokens)-1], "--")
	cmd, _, err := commandTyped(commands.NewApp(), tokens[1:len(tokens)-1])
	if err != nil {
		return "", err
	}

	for _, f := range cmd.Flags {
		stringFlag, ok := f.(*cli.StringFlag)
		if ok && stringFlag.Name == flagName {
			return stringFlag.Value, nil
		}
	}
	return "", fmt.Errorf("%q has no string flag named --%s", line, flagName)
}
