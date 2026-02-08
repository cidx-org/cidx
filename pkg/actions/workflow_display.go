package actions

import (
	"fmt"

	"github.com/cidx-org/cidx/pkg/remote"
)

// DisplayWorkflowStatus renders the current workflow status to stdout
// Used by release, commit-push-watch, and other actions that watch workflows
func DisplayWorkflowStatus(w *remote.Workflow) {
	fmt.Printf("\r\033[K") // Clear line

	for i, job := range w.Jobs {
		var icon string
		switch job.Status {
		case "completed":
			switch job.Conclusion {
			case "success":
				icon = "✓"
			case "skipped":
				icon = "○"
			default:
				icon = "✗"
			}
		case "in_progress":
			icon = "⏳"
		case "queued":
			icon = "○"
		default:
			icon = "?"
		}

		fmt.Printf("[%s] %s", icon, job.Name)
		if i < len(w.Jobs)-1 {
			fmt.Printf(" ")
		}
	}
}
