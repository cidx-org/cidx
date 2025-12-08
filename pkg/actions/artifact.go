package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/remote/github"
	log "github.com/sirupsen/logrus"
)

// ArtifactProvider is the interface for artifact operations
// Only GitHub supports artifacts currently
type ArtifactProvider interface {
	ListArtifacts(ctx context.Context) (*remote.ArtifactStats, error)
	DeleteArtifact(ctx context.Context, artifactID int64) error
	DeleteArtifactsBefore(ctx context.Context, before time.Time) (deleted int, freedBytes int64, err error)
	DeleteAllArtifacts(ctx context.Context) (deleted int, freedBytes int64, err error)
	DeleteExpiredArtifacts(ctx context.Context) (deleted int, freedBytes int64, err error)
}

// ArtifactListAction lists all artifacts
type ArtifactListAction struct {
	provider remote.Provider
	verbose  bool
}

// NewArtifactList creates a new artifact list action
func NewArtifactList(provider remote.Provider, verbose bool) *ArtifactListAction {
	return &ArtifactListAction{
		provider: provider,
		verbose:  verbose,
	}
}

// Execute runs the artifact list action
func (a *ArtifactListAction) Execute(ctx context.Context) error {
	// Check if provider supports artifacts
	ghClient, ok := a.provider.(*github.Client)
	if !ok {
		return fmt.Errorf("artifact management is only supported for GitHub repositories")
	}

	log.Info("📦 Fetching artifacts...")

	stats, err := ghClient.ListArtifacts(ctx)
	if err != nil {
		return err
	}

	if stats.TotalCount == 0 {
		log.Info("No artifacts found")
		return nil
	}

	// Print summary
	fmt.Printf("\n📊 Artifact Summary\n")
	fmt.Printf("   Total: %d artifacts\n", stats.TotalCount)
	fmt.Printf("   Size:  %s\n", formatBytes(stats.TotalSize))
	fmt.Println()

	if a.verbose {
		fmt.Printf("%-50s %-12s %-20s %-10s\n", "NAME", "SIZE", "CREATED", "STATUS")
		fmt.Printf("%s\n", "────────────────────────────────────────────────────────────────────────────────────────────")

		for _, artifact := range stats.Artifacts {
			status := "active"
			if artifact.Expired {
				status = "expired"
			}
			fmt.Printf("%-50s %-12s %-20s %-10s\n",
				truncateString(artifact.Name, 48),
				formatBytes(artifact.SizeInBytes),
				artifact.CreatedAt.Format("2006-01-02 15:04"),
				status,
			)
		}
	} else {
		// Group by workflow name
		byWorkflow := make(map[string]struct {
			count int
			size  int64
		})

		for _, artifact := range stats.Artifacts {
			name := artifact.WorkflowName
			if name == "" {
				name = "Unknown"
			}
			entry := byWorkflow[name]
			entry.count++
			entry.size += artifact.SizeInBytes
			byWorkflow[name] = entry
		}

		fmt.Printf("%-40s %-10s %-12s\n", "WORKFLOW", "COUNT", "SIZE")
		fmt.Printf("%s\n", "────────────────────────────────────────────────────────────────")

		for name, entry := range byWorkflow {
			fmt.Printf("%-40s %-10d %-12s\n",
				truncateString(name, 38),
				entry.count,
				formatBytes(entry.size),
			)
		}
	}

	return nil
}

// ArtifactStatsAction shows artifact storage statistics
type ArtifactStatsAction struct {
	provider remote.Provider
}

// NewArtifactStats creates a new artifact stats action
func NewArtifactStats(provider remote.Provider) *ArtifactStatsAction {
	return &ArtifactStatsAction{
		provider: provider,
	}
}

// Execute runs the artifact stats action
func (a *ArtifactStatsAction) Execute(ctx context.Context) error {
	// Check if provider supports artifacts
	ghClient, ok := a.provider.(*github.Client)
	if !ok {
		return fmt.Errorf("artifact management is only supported for GitHub repositories")
	}

	log.Info("📊 Calculating artifact statistics...")

	stats, err := ghClient.ListArtifacts(ctx)
	if err != nil {
		return err
	}

	// Calculate statistics
	var expiredCount int
	var expiredSize int64
	var activeCount int
	var activeSize int64

	oldest := time.Now()
	newest := time.Time{}

	for _, artifact := range stats.Artifacts {
		if artifact.Expired {
			expiredCount++
			expiredSize += artifact.SizeInBytes
		} else {
			activeCount++
			activeSize += artifact.SizeInBytes
		}

		if artifact.CreatedAt.Before(oldest) {
			oldest = artifact.CreatedAt
		}
		if artifact.CreatedAt.After(newest) {
			newest = artifact.CreatedAt
		}
	}

	fmt.Printf("\n📦 Artifact Storage Statistics\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Total artifacts:    %d\n", stats.TotalCount)
	fmt.Printf("Total size:         %s\n", formatBytes(stats.TotalSize))
	fmt.Printf("\n")
	fmt.Printf("Active artifacts:   %d (%s)\n", activeCount, formatBytes(activeSize))
	fmt.Printf("Expired artifacts:  %d (%s)\n", expiredCount, formatBytes(expiredSize))
	fmt.Printf("\n")

	if stats.TotalCount > 0 {
		fmt.Printf("Oldest artifact:    %s\n", oldest.Format("2006-01-02 15:04"))
		fmt.Printf("Newest artifact:    %s\n", newest.Format("2006-01-02 15:04"))
	}

	// Recommendations
	if expiredCount > 0 {
		fmt.Printf("\n💡 Tip: Run 'cidx action artifact cleanup --expired' to free %s\n", formatBytes(expiredSize))
	}

	return nil
}

// ArtifactCleanupAction deletes artifacts
type ArtifactCleanupAction struct {
	provider   remote.Provider
	deleteAll  bool
	expired    bool
	olderThan  int
	dryRun     bool
}

// NewArtifactCleanup creates a new artifact cleanup action
func NewArtifactCleanup(provider remote.Provider, deleteAll, expired bool, olderThan int, dryRun bool) *ArtifactCleanupAction {
	return &ArtifactCleanupAction{
		provider:  provider,
		deleteAll: deleteAll,
		expired:   expired,
		olderThan: olderThan,
		dryRun:    dryRun,
	}
}

// Execute runs the artifact cleanup action
func (a *ArtifactCleanupAction) Execute(ctx context.Context) error {
	// Check if provider supports artifacts
	ghClient, ok := a.provider.(*github.Client)
	if !ok {
		return fmt.Errorf("artifact management is only supported for GitHub repositories")
	}

	if a.dryRun {
		log.Info("🧪 Dry run mode - no artifacts will be deleted")
	}

	// First, list all artifacts to show what will be deleted
	stats, err := ghClient.ListArtifacts(ctx)
	if err != nil {
		return err
	}

	if stats.TotalCount == 0 {
		log.Info("No artifacts to clean up")
		return nil
	}

	var toDelete []remote.Artifact
	var reason string

	if a.deleteAll {
		toDelete = stats.Artifacts
		reason = "all artifacts"
	} else if a.expired {
		for _, artifact := range stats.Artifacts {
			if artifact.Expired {
				toDelete = append(toDelete, artifact)
			}
		}
		reason = "expired artifacts"
	} else if a.olderThan > 0 {
		cutoff := time.Now().AddDate(0, 0, -a.olderThan)
		for _, artifact := range stats.Artifacts {
			if artifact.CreatedAt.Before(cutoff) {
				toDelete = append(toDelete, artifact)
			}
		}
		reason = fmt.Sprintf("artifacts older than %d days", a.olderThan)
	}

	if len(toDelete) == 0 {
		log.Infof("No %s found", reason)
		return nil
	}

	// Calculate total size to free
	var totalSize int64
	for _, artifact := range toDelete {
		totalSize += artifact.SizeInBytes
	}

	fmt.Printf("\n🗑️  Artifacts to delete (%s):\n", reason)
	fmt.Printf("   Count: %d\n", len(toDelete))
	fmt.Printf("   Size:  %s\n", formatBytes(totalSize))
	fmt.Println()

	if a.dryRun {
		fmt.Printf("%-50s %-12s %-20s\n", "NAME", "SIZE", "CREATED")
		fmt.Printf("%s\n", "──────────────────────────────────────────────────────────────────────────────")

		for _, artifact := range toDelete {
			fmt.Printf("%-50s %-12s %-20s\n",
				truncateString(artifact.Name, 48),
				formatBytes(artifact.SizeInBytes),
				artifact.CreatedAt.Format("2006-01-02 15:04"),
			)
		}

		fmt.Printf("\n💡 Remove --dry-run to delete these artifacts\n")
		return nil
	}

	// Delete artifacts
	log.Infof("🗑️  Deleting %d artifacts...", len(toDelete))

	var deleted int
	var freedBytes int64

	for _, artifact := range toDelete {
		if err := ghClient.DeleteArtifact(ctx, artifact.ID); err != nil {
			log.Warnf("Failed to delete artifact %s: %v", artifact.Name, err)
			continue
		}
		deleted++
		freedBytes += artifact.SizeInBytes
		log.Debugf("Deleted: %s", artifact.Name)
	}

	fmt.Printf("\n✅ Cleanup complete!\n")
	fmt.Printf("   Deleted: %d artifacts\n", deleted)
	fmt.Printf("   Freed:   %s\n", formatBytes(freedBytes))

	return nil
}

// formatBytes formats bytes into human readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// truncateString truncates a string to maxLen and adds ellipsis if needed
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
}
