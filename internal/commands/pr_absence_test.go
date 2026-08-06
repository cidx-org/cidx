package commands

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/branch"
	"github.com/cidx-org/cidx/v2/pkg/remote"
)

// TestPRAbsenceIsNormal covers issue #362.
//
// `cidx pr status` answered `Error: no PR found for branch 'main'` and exit 1
// on the trunk, which is where every session starts and ends. It printed a
// fault for a healthy repository and made the command unusable in an `&&`
// chain or a prompt helper.
func TestPRAbsenceIsNormal(t *testing.T) {
	// What the provider returns when the branch simply has none, wrapped the
	// way the GitHub and GitLab clients wrap it.
	absent := fmt.Errorf("branch main: %w", remote.ErrNoPullRequest)

	tests := []struct {
		name      string
		err       error
		protected bool
		want      bool
	}{
		{
			name:      "no PR on the trunk is where every session sits",
			err:       absent,
			protected: true,
			want:      true,
		},
		{
			name:      "no PR on a feature branch is worth saying",
			err:       absent,
			protected: false,
			want:      false,
		},
		{
			// The failure that made the sentinel necessary: an expired token
			// answers "no PR" as convincingly as an empty list, and means the
			// opposite. Exiting 0 here would call an unreachable repository
			// healthy.
			name:      "a lookup that failed is not an absence, even on the trunk",
			err:       errors.New("failed to list pull requests: 401 Bad credentials"),
			protected: true,
			want:      false,
		},
		{
			name:      "a lookup that failed on a feature branch is still a failure",
			err:       errors.New("failed to list pull requests: connection refused"),
			protected: false,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prAbsenceIsNormal(tt.err, tt.protected); got != tt.want {
				t.Errorf("prAbsenceIsNormal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsProtectedFollowsTheConfiguredList: the trunk is whatever the project
// says it is. A repository whose trunk is `master`, or one that adds `develop`,
// gets the same quiet answer as this one does on `main` (issue #362).
func TestIsProtectedFollowsTheConfiguredList(t *testing.T) {
	manager := branch.NewManager(branch.Config{Protected: []string{"trunk", "develop"}})

	for _, name := range []string{"trunk", "develop"} {
		if !manager.IsProtected(name) {
			t.Errorf("%q is in the configured protected list and was not recognised", name)
		}
	}
	for _, name := range []string{"main", "feat/x"} {
		if manager.IsProtected(name) {
			t.Errorf("%q is not in the configured protected list and was treated as protected", name)
		}
	}
}
