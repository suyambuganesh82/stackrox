package dispatchers

import (
	"github.com/stackrox/rox/generated/internalapi/central"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/complianceoperator/api/v1alpha1"
	"github.com/stackrox/rox/pkg/logging"
	"github.com/stackrox/rox/sensor/kubernetes/eventpipeline/component"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	log = logging.LoggerForModule()
)

// ProfileDispatcher handles compliance operator profile objects
type ProfileDispatcher struct{}

// NewProfileDispatcher creates and returns a new profile dispatcher
func NewProfileDispatcher() *ProfileDispatcher {
	return &ProfileDispatcher{}
}

// ProcessEvent processes a compliance operator profile
func (c *ProfileDispatcher) ProcessEvent(obj, _ interface{}, action central.ResourceAction) *component.ResourceEvent {
	var complianceProfile v1alpha1.Profile

	unstructuredObject, ok := obj.(*unstructured.Unstructured)
	if !ok {
		log.Errorf("Not of type 'unstructured': %T", obj)
		return nil
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObject.Object, &complianceProfile); err != nil {
		log.Errorf("error converting unstructured to compliance profile: %v", err)
		return nil
	}

	protoProfile := &storage.ComplianceOperatorProfile{
		Id:          string(complianceProfile.UID),
		ProfileId:   complianceProfile.ID,
		Name:        complianceProfile.Name,
		Labels:      complianceProfile.Labels,
		Annotations: complianceProfile.Annotations,
		Description: complianceProfile.Description,
	}
	for _, r := range complianceProfile.Rules {
		protoProfile.Rules = append(protoProfile.Rules, &storage.ComplianceOperatorProfile_Rule{
			Name: string(r),
		})
	}

	events := []*central.SensorEvent{
		{
			Id:     protoProfile.GetId(),
			Action: action,
			Resource: &central.SensorEvent_ComplianceOperatorProfile{
				ComplianceOperatorProfile: protoProfile,
			},
		},
	}
	return component.NewResourceEvent(events, nil, nil)
}
