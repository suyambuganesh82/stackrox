package grpc

import (
	"errors"

	"github.com/stackrox/rox/pkg/errox"
	"google.golang.org/grpc/codes"
)

// RoxErrorToGRPCCode translates known sentinel errors to according gRPC codes.
func RoxErrorToGRPCCode(err error) codes.Code {
	switch {
	case err == nil:
		return codes.OK
	case errors.Is(err, errox.AlreadyExists):
		return codes.AlreadyExists
	case errors.Is(err, errox.InvalidArgs):
		return codes.InvalidArgument
	case errors.Is(err, errox.NotFound):
		return codes.NotFound
	case errors.Is(err, errox.ReferencedByAnotherObject):
		return codes.FailedPrecondition
	case errors.Is(err, errox.InvariantViolation):
		return codes.Internal
	case errors.Is(err, errox.NoCredentials):
		return codes.Unauthenticated
	case errors.Is(err, errox.NoValidRole):
		return codes.Unauthenticated
	case errors.Is(err, errox.NotAuthorized):
		return codes.PermissionDenied
	case errors.Is(err, errox.NoAuthzConfigured):
		return codes.Unimplemented
	default:
		return codes.Internal
	}
}
