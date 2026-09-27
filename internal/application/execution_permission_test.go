package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type permissionRepositoryCheck struct{ completed bool }

func (*permissionRepositoryCheck) RegisterPermission(context.Context, IncomingPermission) error {
	return nil
}

func (*permissionRepositoryCheck) ClaimPermission(
	context.Context, PermissionClaim,
) (MutationResult[PermissionResponseResult], bool, error) {
	return MutationResult[PermissionResponseResult]{}, true, nil
}

func (r *permissionRepositoryCheck) CompletePermission(context.Context, string, string, bool, time.Time) error {
	r.completed = true

	return nil
}

type permissionWireCheck struct{ err error }

func (permissionWireCheck) Generation(context.Context, string) (int64, error) { return 1, nil }

func (w permissionWireCheck) RespondPermission(context.Context, string, string, int64) error {
	return w.err
}

func TestPermissionIndeterminateDoesNotComplete(t *testing.T) {
	for _, tc := range []struct {
		name          string
		wireErr       error
		wantCompleted bool
	}{
		{"indeterminate", ErrPermissionDeliveryIndeterminate, false},
		{"write failed", errors.New("closed"), true},
		{"sent", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository := &permissionRepositoryCheck{}
			permission := NewExecutionPermission(repository, permissionWireCheck{err: tc.wireErr}, time.Now)
			_, _ = permission.Respond(context.Background(), PermissionClaim{})

			if repository.completed != tc.wantCompleted {
				t.Fatalf("completed = %v, want %v", repository.completed, tc.wantCompleted)
			}
		})
	}
}
