package resources

import (
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/containerid"
	"github.com/stackrox/rox/pkg/net"
	podUtils "github.com/stackrox/rox/pkg/pods/utils"
	"github.com/stackrox/rox/sensor/common/clusterentities"
	"github.com/stackrox/rox/sensor/common/store"
	v1 "k8s.io/api/core/v1"
)

type endpointManager interface {
	OnDeploymentCreateOrUpdate(deployment store.DeploymentWrap)
	OnDeploymentRemove(deployment store.DeploymentWrap)

	OnServiceCreate(svc store.ServiceWrap)
	OnServiceUpdateOrRemove(namespace string, sel store.Selector)

	OnNodeCreate(node *nodeWrap)
	OnNodeUpdateOrRemove()
}

type endpointManagerImpl struct {
	serviceStore    *serviceStore
	deploymentStore store.DeploymentStore
	podStore        *PodStore
	nodeStore       *nodeStore

	entityStore *clusterentities.Store
}

func newEndpointManager(serviceStore *serviceStore, deploymentStore store.DeploymentStore, podStore *PodStore, nodeStore *nodeStore, entityStore *clusterentities.Store) endpointManager {
	return &endpointManagerImpl{
		serviceStore:    serviceStore,
		deploymentStore: deploymentStore,
		podStore:        podStore,
		nodeStore:       nodeStore,
		entityStore:     entityStore,
	}
}

func (m *endpointManagerImpl) addEndpointDataForContainerPort(podIP, podHostIP net.IPAddress, node *nodeWrap, port v1.ContainerPort, data *clusterentities.EntityData) {
	l4Proto := convertL4Proto(port.Protocol)
	targetInfo := clusterentities.EndpointTargetInfo{
		ContainerPort: uint16(port.ContainerPort),
		PortName:      port.Name,
	}

	if podIP.IsValid() {
		podEndpoint := net.MakeNumericEndpoint(podIP, uint16(port.ContainerPort), l4Proto)
		data.AddEndpoint(podEndpoint, targetInfo)
	}

	if port.HostPort != 0 {
		var hostIPs []net.IPAddress
		boundHostIP := net.ParseIP(port.HostIP)
		if !boundHostIP.IsValid() || boundHostIP.IsUnspecified() {
			if node != nil {
				hostIPs = node.addresses
			} else if podHostIP.IsValid() {
				hostIPs = []net.IPAddress{podHostIP}
			}
		} else if !boundHostIP.IsLoopback() {
			hostIPs = []net.IPAddress{boundHostIP}
		}

		for _, hostIP := range hostIPs {
			hostEndpoint := net.MakeNumericEndpoint(hostIP, uint16(port.HostPort), l4Proto)
			data.AddEndpoint(hostEndpoint, targetInfo)
		}
	}
}

func (m *endpointManagerImpl) addEndpointDataForPod(pod *v1.Pod, data *clusterentities.EntityData) {
	podIP := net.ParseIP(pod.Status.PodIP)
	// Do not register the pod if it is using the host network (i.e., pod IP = node IP), as this causes issues with
	// kube-proxy connections.
	if !pod.Spec.HostNetwork && podIP.IsValid() {
		data.AddIP(podIP)
	}

	var node *nodeWrap
	if pod.Spec.NodeName != "" {
		node = m.nodeStore.getNode(pod.Spec.NodeName)
	}
	podHostIP := net.ParseIP(pod.Status.HostIP)

	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			m.addEndpointDataForContainerPort(podIP, podHostIP, node, port, data)
		}
	}
}

func (m *endpointManagerImpl) endpointDataForDeployment(w store.DeploymentWrap) *clusterentities.EntityData {
	result := &clusterentities.EntityData{}

	for _, pod := range w.GetPods() {
		m.addEndpointDataForPod(pod, result)
	}

	for _, svc := range m.serviceStore.GetMatchingServicesWithRoutes(w.GetNamespace(), w.GetPodLabels()) {
		m.addEndpointDataForService(w, svc.GetServiceWrap(), result)
	}

	m.podStore.forEach(w.GetNamespace(), w.GetId(), func(p *storage.Pod) {
		for _, inst := range p.GetLiveInstances() {
			id := containerid.ShortContainerIDFromInstance(inst)
			if id == "" {
				continue
			}
			podID := inst.GetContainingPodId()
			if id, err := podUtils.ParsePodID(podID); err == nil {
				podID = id.Name
			}

			result.AddContainerID(id, clusterentities.ContainerMetadata{
				DeploymentID:  w.GetId(),
				DeploymentTS:  w.GetStateTimestamp(),
				PodID:         podID,
				PodUID:        p.GetId(),
				ContainerName: inst.GetContainerName(),
				ContainerID:   id,
				Namespace:     w.GetNamespace(),
				StartTime:     inst.GetStarted(),
				ImageID:       inst.GetImageDigest(),
			})
		}
	})

	return result
}

func getAllServiceIPs(svc *v1.Service) (serviceIPs []net.IPAddress) {
	if clusterIP := net.ParseIP(svc.Spec.ClusterIP); clusterIP.IsValid() {
		serviceIPs = append(serviceIPs, clusterIP)
	}
	for _, extIPStr := range svc.Spec.ExternalIPs {
		if extIP := net.ParseIP(extIPStr); extIP.IsValid() {
			serviceIPs = append(serviceIPs, extIP)
		}
	}
	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		for _, ingressLB := range svc.Status.LoadBalancer.Ingress {
			if lbIP := net.ParseIP(ingressLB.IP); lbIP.IsValid() {
				serviceIPs = append(serviceIPs, lbIP)
			}
		}
	}
	return
}

func addEndpointDataForServicePort(deployment store.DeploymentWrap, serviceIPs []net.IPAddress, nodeIPs []net.IPAddress, port v1.ServicePort, data *clusterentities.EntityData) {
	l4Proto := convertL4Proto(port.Protocol)

	targetInfo := clusterentities.EndpointTargetInfo{
		PortName: port.Name,
	}
	if portCfg := deployment.GetPortConfigs()[portRefOf(port)]; portCfg != nil {
		targetInfo.ContainerPort = uint16(portCfg.ContainerPort)
	} else {
		targetInfo.ContainerPort = uint16(port.TargetPort.IntValue())
	}

	for _, serviceIP := range serviceIPs {
		serviceEndpoint := net.MakeNumericEndpoint(serviceIP, uint16(port.Port), l4Proto)
		data.AddEndpoint(serviceEndpoint, targetInfo)
	}

	if port.NodePort != 0 {
		for _, nodeIP := range nodeIPs {
			nodePortEndpoint := net.MakeNumericEndpoint(nodeIP, uint16(port.NodePort), l4Proto)
			data.AddEndpoint(nodePortEndpoint, targetInfo)
		}
	}
}

func (m *endpointManagerImpl) addEndpointDataForService(deployment store.DeploymentWrap, svc store.ServiceWrap, data *clusterentities.EntityData) {
	var allNodeIPs []net.IPAddress
	if svc.GetSpec().Type == v1.ServiceTypeLoadBalancer || svc.GetSpec().Type == v1.ServiceTypeNodePort {
		for _, node := range m.nodeStore.getNodes() {
			allNodeIPs = append(allNodeIPs, node.addresses...)
		}
	}

	serviceIPs := getAllServiceIPs(svc.GetService())
	for _, port := range svc.GetSpec().Ports {
		addEndpointDataForServicePort(deployment, serviceIPs, allNodeIPs, port, data)
	}
}

func (m *endpointManagerImpl) OnServiceCreate(svc store.ServiceWrap) {
	updates := make(map[string]*clusterentities.EntityData)
	for _, deployment := range m.deploymentStore.GetMatchingDeployments(svc.GetNamespace(), svc.GetSelector()) {
		update := &clusterentities.EntityData{}
		m.addEndpointDataForService(deployment, svc, update)
		updates[deployment.GetId()] = update
	}

	m.entityStore.Apply(updates, true)
}

func (m *endpointManagerImpl) OnServiceUpdateOrRemove(namespace string, sel store.Selector) {
	updates := make(map[string]*clusterentities.EntityData)
	for _, deployment := range m.deploymentStore.GetMatchingDeployments(namespace, sel) {
		updates[deployment.GetId()] = m.endpointDataForDeployment(deployment)
	}

	m.entityStore.Apply(updates, false)
}

func (m *endpointManagerImpl) OnNodeCreate(node *nodeWrap) {
	if len(node.addresses) == 0 {
		return
	}

	updates := make(map[string]*clusterentities.EntityData)
	for _, svc := range m.serviceStore.NodePortServicesSnapshot() {
		for _, deployment := range m.deploymentStore.GetMatchingDeployments(svc.Namespace, svc.selector) {
			update, ok := updates[deployment.GetId()]
			if !ok {
				update = &clusterentities.EntityData{}
				updates[deployment.GetId()] = update
			}
			for _, port := range svc.Spec.Ports {
				if port.NodePort != 0 {
					addEndpointDataForServicePort(deployment, nil, node.addresses, port, update)
				}
			}
		}
	}

	m.entityStore.Apply(updates, true)
}

func (m *endpointManagerImpl) OnNodeUpdateOrRemove() {
	affectedDeployments := make(map[store.DeploymentWrap]struct{})

	for _, svc := range m.serviceStore.NodePortServicesSnapshot() {
		for _, deployment := range m.deploymentStore.GetMatchingDeployments(svc.Namespace, svc.selector) {
			affectedDeployments[deployment] = struct{}{}
		}
	}

	updates := make(map[string]*clusterentities.EntityData, len(affectedDeployments))
	for deployment := range affectedDeployments {
		updates[deployment.GetId()] = m.endpointDataForDeployment(deployment)
	}

	m.entityStore.Apply(updates, false)
}

func (m *endpointManagerImpl) OnDeploymentCreateOrUpdate(deployment store.DeploymentWrap) {
	updates := map[string]*clusterentities.EntityData{
		deployment.GetId(): m.endpointDataForDeployment(deployment),
	}
	m.entityStore.Apply(updates, false)
}

func (m *endpointManagerImpl) OnDeploymentRemove(deployment store.DeploymentWrap) {
	updates := map[string]*clusterentities.EntityData{
		deployment.GetId(): nil,
	}
	m.entityStore.Apply(updates, false)
}

func convertL4Proto(proto v1.Protocol) net.L4Proto {
	switch proto {
	case v1.ProtocolTCP:
		return net.TCP
	case v1.ProtocolUDP:
		return net.UDP
	default:
		return net.L4Proto(-1)
	}
}
