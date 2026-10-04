package bdd

import (
	"testing"

	"github.com/cucumber/godog"
)

func InitializeScenario(sc *godog.ScenarioContext) {
	tc := newTestContext()

	registerOnDemandSteps(sc, tc)
	registerSubscriptionSteps(sc, tc)
	registerProviderSteps(sc, tc)
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("BDD scenarios failed")
	}
}
