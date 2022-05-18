// Code generated by genny. DO NOT EDIT.
// This file was automatically generated by genny.
// Any changes will be lost if this file is regenerated.
// see https://github.com/mauricelam/genny

package fixtures

import (
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/sac/testconsts"
	"github.com/stackrox/rox/pkg/uuid"
)

// *storage.Pod represents a generic type that we use in the function below.

// GetSACTestStoragePodSet returns a set of mock *storage.Pod that can be used
// for scoped access control sets.
// It will include:
// 9 *storage.Pod scoped to Cluster1, 3 to each Namespace A / B / C.
// 9 *storage.Pod scoped to Cluster2, 3 to each Namespace A / B / C.
// 9 *storage.Pod scoped to Cluster3, 3 to each Namespace A / B / C.
func GetSACTestStoragePodSet(scopedStoragePodCreator func(id string, clusterID string, namespace string) *storage.Pod) []*storage.Pod {
	return []*storage.Pod{
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceC),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceC),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster1, testconsts.NamespaceC),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceC),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceC),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster2, testconsts.NamespaceC),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceA),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceB),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceC),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceC),
		scopedStoragePodCreator(uuid.NewV4().String(), testconsts.Cluster3, testconsts.NamespaceC),
	}
}
