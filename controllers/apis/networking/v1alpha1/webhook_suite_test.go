package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkloadsMutatingWebhooks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Mutating Webhooks Unit Test Suite")
}
