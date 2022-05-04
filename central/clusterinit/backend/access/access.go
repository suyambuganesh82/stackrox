package access

import (
	"context"

	"github.com/pkg/errors"
	"github.com/stackrox/rox/central/role/resources"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/errox"
	"github.com/stackrox/rox/pkg/sac"
)

// CheckAccess returns nil if requested access level is granted in context.
func CheckAccess(ctx context.Context, access storage.Access) error {
	// we need access to the API token and service identity resources
	scopes := [][]sac.ScopeKey{
		{sac.AccessModeScopeKey(access), sac.ResourceScopeKey(resources.APIToken.GetResource())},
		{sac.AccessModeScopeKey(access), sac.ResourceScopeKey(resources.ServiceIdentity.GetResource())},
	}
	if allowed, err := sac.GlobalAccessScopeChecker(ctx).AllAllowed(ctx, scopes); err != nil {
		return errors.Wrap(err, "checking access")
	} else if !allowed {
		return errox.NotAuthorized
	}
	return nil
}
