// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package pluginregistry

import (
	"fmt"
	"strings"
	"sync"

	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

var log = logging.GetLogger("pkg/pluginregistry")

// PluginRegistry manages all the plugins available in the system. It associates Plugin
// identifiers (implemented as simple strings) with factory functions that create instances
// of those plugins. A Plugin instance is owner by a single Job object.
type PluginRegistry struct {
	lock sync.RWMutex

	// TargetManagers collects a mapping of Plugin Name <-> TargetManager constructor
	TargetManagers map[string]target.TargetManagerFactory

	// TestFetchers collects a mapping of Plugin Name <-> TestFetchers constructor
	TestFetchers map[string]test.TestFetcherFactory

	// Step collects a mapping of Plugin Name <-> Step constructor
	Steps map[string]test.StepFactory

	// StepEvents collects a mapping between Step and list of event.Name
	// that the Step is allowed to emit at runtime
	StepsEvents map[string]map[event.Name]bool

	// Reporters collects a mapping of Plugin Name <-> Reporter constructor
	Reporters map[string]job.ReporterFactory
}

// NewPluginRegistry constructs a new empty plugin registry
func NewPluginRegistry() *PluginRegistry {
	pr := PluginRegistry{}
	pr.TargetManagers = make(map[string]target.TargetManagerFactory)
	pr.TestFetchers = make(map[string]test.TestFetcherFactory)
	pr.Steps = make(map[string]test.StepFactory)
	pr.StepsEvents = make(map[string]map[event.Name]bool)
	pr.Reporters = make(map[string]job.ReporterFactory)
	return &pr
}

// RegisterTargetManager register a factory for TargetManager plugins
func (r *PluginRegistry) RegisterTargetManager(pluginName string, tmf target.TargetManagerFactory) error {
	pluginName = strings.ToLower(pluginName)
	r.lock.Lock()
	defer r.lock.Unlock()
	log.Infof("Registering target manager %s", pluginName)
	if _, found := r.TargetManagers[pluginName]; found {
		return fmt.Errorf("TargetManager %s already registered", pluginName)
	}
	r.TargetManagers[pluginName] = tmf
	return nil
}

// RegisterTestFetcher registers a TestFetcher within the registry
func (r *PluginRegistry) RegisterTestFetcher(pluginName string, tff test.TestFetcherFactory) error {
	pluginName = strings.ToLower(pluginName)
	r.lock.Lock()
	defer r.lock.Unlock()
	log.Infof("Registering test fetcher %s", pluginName)
	if _, found := r.TestFetchers[pluginName]; found {
		return fmt.Errorf("TestFetcher %s already registered", pluginName)
	}
	r.TestFetchers[pluginName] = tff
	return nil
}

// RegisterStep registers a Step within the registry and the associated events
func (r *PluginRegistry) RegisterStep(pluginName string, tsf test.StepFactory, stepEvents []event.Name) error {
	pluginName = strings.ToLower(pluginName)
	r.lock.Lock()
	defer r.lock.Unlock()
	log.Infof("Registering test step %s", pluginName)
	if _, found := r.Steps[pluginName]; found {
		return fmt.Errorf("Steps %s already registered", pluginName)
	}
	r.Steps[pluginName] = tsf

	// Verify that all the events the test step is associated with validate correctly
	mapEvents := make(map[event.Name]bool)

	for _, event := range stepEvents {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("could not register Step %s: %v", pluginName, err)
		}
		mapEvents[event] = true
	}
	r.StepsEvents[pluginName] = mapEvents
	return nil
}

// RegisterReporter registers a Reporter within the registry
func (r *PluginRegistry) RegisterReporter(pluginName string, rf job.ReporterFactory) error {
	pluginName = strings.ToLower(pluginName)
	r.lock.Lock()
	defer r.lock.Unlock()
	log.Infof("Registering reporter %s", pluginName)
	if _, found := r.Reporters[pluginName]; found {
		return fmt.Errorf("Reporter %s already registered", pluginName)
	}
	r.Reporters[pluginName] = rf
	return nil
}

// NewTargetManager returns a new instance of TargetManager from its
// corresponding name
func (r *PluginRegistry) NewTargetManager(pluginName string) (target.TargetManager, error) {
	pluginName = strings.ToLower(pluginName)
	var (
		targetManagerFactory target.TargetManagerFactory
		found                bool
	)
	r.lock.RLock()
	targetManagerFactory, found = r.TargetManagers[pluginName]
	r.lock.RUnlock()
	if !found {
		return nil, fmt.Errorf("TargetManager %v is not registered", pluginName)
	}
	targetManager := targetManagerFactory()
	return targetManager, nil
}

// NewTestFetcher returns a new instance of TestFetcher from its
// corresponding name
func (r *PluginRegistry) NewTestFetcher(pluginName string) (test.TestFetcher, error) {
	pluginName = strings.ToLower(pluginName)
	var (
		testFetcherFactory test.TestFetcherFactory
		found              bool
	)
	r.lock.RLock()
	testFetcherFactory, found = r.TestFetchers[pluginName]
	r.lock.RUnlock()
	if !found {
		return nil, fmt.Errorf("TestFetcher %v is not registered", pluginName)
	}
	testFetcher := testFetcherFactory()
	return testFetcher, nil
}

// NewStep returns a new instance of a Step from its corresponding name
func (r *PluginRegistry) NewStep(pluginName string) (test.Step, error) {
	pluginName = strings.ToLower(pluginName)
	var (
		testStepFactory test.StepFactory
		found           bool
	)
	r.lock.RLock()
	testStepFactory, found = r.Steps[pluginName]
	r.lock.RUnlock()
	if !found {
		return nil, fmt.Errorf("Step %s is not registered", pluginName)
	}
	testStep := testStepFactory()
	return testStep, nil
}

// NewStepEvents returns a map of events.EventName which can be emitted by the Step
func (r *PluginRegistry) NewStepEvents(pluginName string) (map[event.Name]bool, error) {
	pluginName = strings.ToLower(pluginName)
	var (
		testStepEvents map[event.Name]bool
		found          bool
	)
	r.lock.RLock()
	testStepEvents, found = r.StepsEvents[pluginName]
	r.lock.RUnlock()
	if !found {
		return nil, fmt.Errorf("Step %s does not have any event associated", pluginName)
	}
	return testStepEvents, nil
}

// NewReporter returns a new instance of a Reporter from its corresponding name
func (r *PluginRegistry) NewReporter(pluginName string) (job.Reporter, error) {
	pluginName = strings.ToLower(pluginName)
	var (
		reporterFactory job.ReporterFactory
		found           bool
	)
	r.lock.RLock()
	reporterFactory, found = r.Reporters[pluginName]
	r.lock.RUnlock()
	if !found {
		return nil, fmt.Errorf("Reporter %s is not registered", pluginName)
	}
	reporter := reporterFactory()
	return reporter, nil
}
