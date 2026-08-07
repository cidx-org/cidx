package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/google/go-github/v76/github"
)

// ListArtifacts returns all artifacts for the repository with storage statistics
func (c *Client) ListArtifacts(ctx context.Context) (*remote.ArtifactStats, error) {
	stats := &remote.ArtifactStats{
		Artifacts: []remote.Artifact{},
	}

	opts := &github.ListArtifactsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		artifacts, resp, err := c.client.Actions.ListArtifacts(ctx, c.owner, c.repo, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list artifacts: %w", err)
		}

		for _, a := range artifacts.Artifacts {
			artifact := remote.Artifact{
				ID:          a.GetID(),
				Name:        a.GetName(),
				SizeInBytes: a.GetSizeInBytes(),
				CreatedAt:   a.GetCreatedAt().Time,
				ExpiresAt:   a.GetExpiresAt().Time,
				Expired:     a.GetExpired(),
			}

			// Get workflow run info if available
			if a.WorkflowRun != nil {
				artifact.WorkflowRun = strconv.FormatInt(a.WorkflowRun.GetID(), 10)
				// WorkflowRun doesn't have GetName, use head branch instead
				artifact.WorkflowName = a.WorkflowRun.GetHeadBranch()
			}

			stats.Artifacts = append(stats.Artifacts, artifact)
			stats.TotalSize += a.GetSizeInBytes()
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	stats.TotalCount = len(stats.Artifacts)
	return stats, nil
}

// ListRunArtifacts returns the artifacts one run produced.
//
// The repository-wide ListArtifacts above cannot answer this: the audit uploads
// one artifact per matrix leg, so "the trivy results" is a set of twelve that
// only means anything taken from the same run (issue #285).
func (c *Client) ListRunArtifacts(ctx context.Context, runID string) ([]remote.Artifact, error) {
	id, err := strconv.ParseInt(runID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow run ID %q: %w", runID, err)
	}

	var artifacts []remote.Artifact
	opts := &github.ListOptions{PerPage: 100}

	for {
		list, resp, err := c.client.Actions.ListWorkflowRunArtifacts(ctx, c.owner, c.repo, id, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list artifacts of run %s: %w", runID, err)
		}

		for _, a := range list.Artifacts {
			artifacts = append(artifacts, remote.Artifact{
				ID:          a.GetID(),
				Name:        a.GetName(),
				SizeInBytes: a.GetSizeInBytes(),
				CreatedAt:   a.GetCreatedAt().Time,
				ExpiresAt:   a.GetExpiresAt().Time,
				Expired:     a.GetExpired(),
				WorkflowRun: runID,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return artifacts, nil
}

// DownloadArtifact opens the zip archive of one artifact.
//
// GitHub answers the artifact endpoint with a 302 to a pre-signed blob URL that
// carries its own credentials, so the second request is a plain GET: sending the
// token there would hand it to a storage host that never asked for it.
func (c *Client) DownloadArtifact(ctx context.Context, artifactID int64) (io.ReadCloser, error) {
	location, _, err := c.client.Actions.DownloadArtifact(ctx, c.owner, c.repo, artifactID, maxArtifactRedirects)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the download URL of artifact %d: %w", artifactID, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build the download request for artifact %d: %w", artifactID, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact %d: %w", artifactID, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("failed to download artifact %d: %s", artifactID, resp.Status)
	}

	return resp.Body, nil
}

// maxArtifactRedirects is what go-github follows before giving up. One redirect
// is what the API documents; the margin covers a storage host that adds its own.
const maxArtifactRedirects = 5

// DeleteArtifact deletes a single artifact by ID
func (c *Client) DeleteArtifact(ctx context.Context, artifactID int64) error {
	_, err := c.client.Actions.DeleteArtifact(ctx, c.owner, c.repo, artifactID)
	if err != nil {
		return fmt.Errorf("failed to delete artifact %d: %w", artifactID, err)
	}
	return nil
}

// DeleteArtifactsBefore deletes all artifacts created before the given time
func (c *Client) DeleteArtifactsBefore(ctx context.Context, before time.Time) (deleted int, freedBytes int64, err error) {
	stats, err := c.ListArtifacts(ctx)
	if err != nil {
		return 0, 0, err
	}

	for _, artifact := range stats.Artifacts {
		if artifact.CreatedAt.Before(before) {
			if err := c.DeleteArtifact(ctx, artifact.ID); err != nil {
				return deleted, freedBytes, fmt.Errorf("failed to delete artifact %s: %w", artifact.Name, err)
			}
			deleted++
			freedBytes += artifact.SizeInBytes
		}
	}

	return deleted, freedBytes, nil
}

// DeleteAllArtifacts deletes all artifacts in the repository
func (c *Client) DeleteAllArtifacts(ctx context.Context) (deleted int, freedBytes int64, err error) {
	stats, err := c.ListArtifacts(ctx)
	if err != nil {
		return 0, 0, err
	}

	for _, artifact := range stats.Artifacts {
		if err := c.DeleteArtifact(ctx, artifact.ID); err != nil {
			return deleted, freedBytes, fmt.Errorf("failed to delete artifact %s: %w", artifact.Name, err)
		}
		deleted++
		freedBytes += artifact.SizeInBytes
	}

	return deleted, freedBytes, nil
}

// DeleteExpiredArtifacts deletes all expired artifacts
func (c *Client) DeleteExpiredArtifacts(ctx context.Context) (deleted int, freedBytes int64, err error) {
	stats, err := c.ListArtifacts(ctx)
	if err != nil {
		return 0, 0, err
	}

	for _, artifact := range stats.Artifacts {
		if artifact.Expired {
			if err := c.DeleteArtifact(ctx, artifact.ID); err != nil {
				return deleted, freedBytes, fmt.Errorf("failed to delete artifact %s: %w", artifact.Name, err)
			}
			deleted++
			freedBytes += artifact.SizeInBytes
		}
	}

	return deleted, freedBytes, nil
}
