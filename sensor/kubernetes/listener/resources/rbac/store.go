package rbac

import (
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/sensor/common/rbac"
	v1 "k8s.io/api/rbac/v1"
)

// Store handles correlating updates to K8s rbac types and generates events from them.
type Store interface {
	GetNamespacedRoleIDOrEmpty(roleRef namespacedRoleRef) string

	UpsertRole(role *v1.Role)
	RemoveRole(role *v1.Role)

	UpsertClusterRole(role *v1.ClusterRole)
	RemoveClusterRole(role *v1.ClusterRole)

	UpsertBinding(binding *v1.RoleBinding)
	RemoveBinding(binding *v1.RoleBinding)

	UpsertClusterBinding(binding *v1.ClusterRoleBinding)
	RemoveClusterBinding(binding *v1.ClusterRoleBinding)
	GetPermissionLevelForDeployment(deployment rbac.NamespacedServiceAccount) storage.PermissionLevel
}

// NewStore creates a new instance of Store
func NewStore() Store {
	return &storeImpl{
		roles:    make(map[namespacedRoleRef]namespacedRole),
		bindings: make(map[namespacedBindingID]*namespacedBinding),

		// Incredibly unlikely that there are no roles and no bindings, but for safety initialize empty buckets
		bucketEvaluator: newBucketEvaluator(nil, nil),
	}
}
