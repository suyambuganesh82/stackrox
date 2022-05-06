package queue

import (
	"container/list"

	"github.com/gogo/protobuf/types"
	"github.com/stackrox/rox/pkg/sync"
)

// DeploymentObservation struct used as element in the queue
type DeploymentObservation struct {
	DeploymentID   string
	DeploymentName string
	ClusterID      string
	Namespace      string
	InObservation  bool
	ObservationEnd *types.Timestamp
}

//go:generate mockgen-wrapper
// DeploymentObservationQueue interface for observation queue
type DeploymentObservationQueue interface {
	InObservation(deploymentID string) bool
	Pull() *DeploymentObservation
	Peek() *DeploymentObservation
	Push(observation *DeploymentObservation)
	RemoveDeployment(deploymentID string)
	RemoveDeploymentsForCluster(clusterID string)
	RemoveFromObservation(deploymentID string)
	GetObservationDetails(deploymentID string) *DeploymentObservation
}

// deploymentObservationQueue queue for deployments in observation window
type deploymentObservationQueueImpl struct {
	mutex                sync.Mutex
	queue                *list.List
	deploymentMap        map[string]*list.Element
	deploymentClusterMap map[string]string
}

// New creates a new instance of the queue
func New() DeploymentObservationQueue {
	return &deploymentObservationQueueImpl{
		queue:                list.New(),
		deploymentMap:        make(map[string]*list.Element),
		deploymentClusterMap: make(map[string]string),
	}
}

// InObservation returns if this deployment is still in the observation window
func (q *deploymentObservationQueueImpl) InObservation(deploymentID string) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	deployMap, found := q.deploymentMap[deploymentID]

	// If the deployment is found AND the map object is nil then we are no longer observing this deployment.
	// Thus if (found && deployMap == nil) evaluates to true, then we want to return false.
	return !(found && deployMap == nil)
}

// Pull pulls an element from the deployment queue
func (q *deploymentObservationQueueImpl) Pull() *DeploymentObservation {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.queue.Len() == 0 {
		return nil
	}

	dep := q.queue.Remove(q.queue.Front()).(*DeploymentObservation)

	// Keep the deployment in the map, so we know that we have processed this deployment.
	q.deploymentMap[dep.DeploymentID] = nil

	return dep
}

// Peek returns the first item in the list without removing it
func (q *deploymentObservationQueueImpl) Peek() *DeploymentObservation {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.queue.Len() == 0 {
		return nil
	}

	return q.queue.Front().Value.(*DeploymentObservation)
}

// Push attempts to add an item to the queue, and does nothing if object already exists.
func (q *deploymentObservationQueueImpl) Push(observation *DeploymentObservation) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// already observing or observed this deployment
	if _, found := q.deploymentMap[observation.DeploymentID]; found {
		return
	}
	depObj := q.queue.PushBack(observation)
	// Reference the list object in the deployment map
	q.deploymentMap[observation.DeploymentID] = depObj

	// If caller is using cluster, add it to deployment Cluster map
	if len(observation.ClusterID) > 0 {
		q.deploymentClusterMap[observation.DeploymentID] = observation.ClusterID
	}

}

// removeListItem removes the list item associated with a deployment
func (q *deploymentObservationQueueImpl) removeListItem(deploymentID string) {
	// The deployment is kept in the map after it has been processed to ensure we
	// do not process it again.  In that case the depObj will be nil
	depObj, found := q.deploymentMap[deploymentID]
	if !found {
		return
	}

	// Remove the object from the queue if it is not nil.
	if depObj != nil {
		q.queue.Remove(depObj)
	}
}

// RemoveDeployment removes a deployment from the list and the map
func (q *deploymentObservationQueueImpl) RemoveDeployment(deploymentID string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// remove the corresponding list items
	q.removeListItem(deploymentID)

	delete(q.deploymentMap, deploymentID)
	delete(q.deploymentClusterMap, deploymentID)
}

// RemoveDeploymentsForCluster removes all deployments associated with a cluster from the list and the map
func (q *deploymentObservationQueueImpl) RemoveDeploymentsForCluster(clusterID string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for deploymentID, mappedClusterID := range q.deploymentClusterMap {
		if clusterID == mappedClusterID {
			// remove the corresponding list items
			q.removeListItem(deploymentID)

			delete(q.deploymentMap, deploymentID)
			delete(q.deploymentClusterMap, deploymentID)
		}
	}
}

// RemoveFromObservation removes a deployment from observation
func (q *deploymentObservationQueueImpl) RemoveFromObservation(deploymentID string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// The deployment is kept in the map after it has been processed to ensure we
	// do not process it again.  In that case the depObj will be nil
	depObj, found := q.deploymentMap[deploymentID]
	if !found {
		return
	}

	// Remove the object from the queue if it is not nil.
	if depObj != nil {
		q.queue.Remove(depObj)
	}
	// Keep the deployment in the map, so we know that we have processed this deployment.
	q.deploymentMap[deploymentID] = nil
}

// GetObservationDetails gets the observations details of the deployment
func (q *deploymentObservationQueueImpl) GetObservationDetails(deploymentID string) *DeploymentObservation {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// The deployment is kept in the map after it has been processed to ensure we
	// do not process it again.  In that case the depObj will be nil
	depObj, found := q.deploymentMap[deploymentID]
	if !found || depObj == nil {
		return nil
	}

	return depObj.Value.(*DeploymentObservation)
}
