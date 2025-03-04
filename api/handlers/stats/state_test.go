package stats_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/api/handlers/stats/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("State", func() {
	var (
		processRepo    *fake.CFProcessRepository
		stateCollector *stats.ProcessInstancesStateCollector
		instancesState []stats.ProcessInstanceState
		stateErr       error
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		processRepo.GetProcessReturns(repositories.ProcessRecord{
			Type: "web",
			InstancesStatus: map[string]korifiv1alpha1.InstanceStatus{
				"1": {
					State: korifiv1alpha1.InstanceStateCrashed,
				},
				"2": {
					State:     korifiv1alpha1.InstanceStateRunning,
					Timestamp: tools.PtrTo(metav1.NewTime(time.UnixMilli(2000).UTC())),
				},
			},
		}, nil)

		stateCollector = stats.NewProcessInstanceStateCollector(processRepo)
	})

	JustBeforeEach(func() {
		instancesState, stateErr = stateCollector.CollectProcessInstancesStates(ctx, "process-guid")
	})

	It("gets the process from the repository", func() {
		Expect(stateErr).NotTo(HaveOccurred())

		Expect(processRepo.GetProcessCallCount()).To(Equal(1))
		_, actualAuthInfo, actualProcessGUID := processRepo.GetProcessArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(actualProcessGUID).To(Equal("process-guid"))
	})

	When("getting the process fails", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("get-process-err"))
		})

		It("returns an error", func() {
			Expect(stateErr).To(MatchError(ContainSubstring("get-process-err")))
		})
	})

	It("returns instances state", func() {
		Expect(stateErr).NotTo(HaveOccurred())
		Expect(instancesState).To(ConsistOf(
			stats.ProcessInstanceState{
				ID:    1,
				Type:  "web",
				State: korifiv1alpha1.InstanceStateCrashed,
			},
			stats.ProcessInstanceState{
				ID:        2,
				Type:      "web",
				State:     korifiv1alpha1.InstanceStateRunning,
				Timestamp: tools.PtrTo(metav1.NewTime(time.UnixMilli(2000).UTC())),
			},
		))
	})

	When("instance id is not a number", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				Type: "web",
				InstancesStatus: map[string]korifiv1alpha1.InstanceStatus{
					"one": {
						State: korifiv1alpha1.InstanceStateCrashed,
					},
				},
			}, nil)
		})

		It("returns an error", func() {
			Expect(stateErr).To(MatchError(ContainSubstring("one")))
		})
	})
})
