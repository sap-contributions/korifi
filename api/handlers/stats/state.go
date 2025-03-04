package stats

import (
	"context"
	"fmt"
	"strconv"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProcessInstanceState struct {
	ID        int
	Type      string
	State     korifiv1alpha1.InstanceState
	Timestamp *metav1.Time
}

//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository
type CFProcessRepository interface {
	GetProcess(context.Context, authorization.Info, string) (repositories.ProcessRecord, error)
}

type ProcessInstancesStateCollector struct {
	processRepo CFProcessRepository
}

func NewProcessInstanceStateCollector(processRepo CFProcessRepository) *ProcessInstancesStateCollector {
	return &ProcessInstancesStateCollector{
		processRepo: processRepo,
	}
}

func (c *ProcessInstancesStateCollector) CollectProcessInstancesStates(ctx context.Context, processGUID string) ([]ProcessInstanceState, error) {
	authInfo, _ := authorization.InfoFromContext(ctx)

	process, err := c.processRepo.GetProcess(ctx, authInfo, processGUID)
	if err != nil {
		return nil, err
	}

	states := []ProcessInstanceState{}
	for instanceId, instanceStatus := range process.InstancesStatus {
		instanceIdInt, err := strconv.Atoi(instanceId)
		if err != nil {
			return nil, fmt.Errorf("parsing instance id %q failed: %w", instanceId, err)
		}

		states = append(states, ProcessInstanceState{
			ID:        instanceIdInt,
			Type:      process.Type,
			State:     instanceStatus.State,
			Timestamp: instanceStatus.Timestamp,
		})

	}

	return states, nil
}
