package github

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
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
