package integration_test

import (
	"os"

	"github.com/pivotal-cf-experimental/go-pivnet"
	"github.com/pivotal-golang/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

const testProductSlug = "pivnet-resource-test"

var client pivnet.Client

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	APIToken := os.Getenv("API_TOKEN")
	Endpoint := os.Getenv("ENDPOINT")

	if APIToken == "" {
		Fail("API_TOKEN must be set for integration tests to run")
	}

	if Endpoint == "" {
		Fail("Endpoint must be set for integration tests to run")
	}

	config := pivnet.ClientConfig{
		Endpoint:  Endpoint,
		Token:     APIToken,
		UserAgent: "go-pivnet/integration-test",
	}

	logger := lager.NewLogger("integration tests")
	logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

	client = pivnet.NewClient(config, logger)
})