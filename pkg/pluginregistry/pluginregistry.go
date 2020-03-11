// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package pluginregistry

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/facebookincubator/contest/pkg/abstract"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
)

var log = logging.GetLogger("pkg/pluginregistry")

// FactoryType is the factory type selector.
//
// Each plugin contains a factory to construct an object which
// implements some feature (target.Locker, test.TestStep or any other feature).
// And it is possible to request a factory/factories of a specific type via
// methods `(*PluginRegistry).Factory` and `(*PluginRegistry).Factories`.
type FactoryType int

const (
	// FactoryTypeUndefined is the zero-value of FactoryType
	FactoryTypeUndefined = FactoryType(iota)

	// FactoryTypeTargetLocker corresponds to factory of target.Locker
	FactoryTypeTargetLocker

	// FactoryTypeTargetManager corresponds to factory of target.TargetManager
	FactoryTypeTargetManager

	// FactoryTypeJobReporter corresponds to factory of job.Reporter
	FactoryTypeJobReporter

	// FactoryTypeTestStep corresponds to factory of test.TestStep
	FactoryTypeTestStep

	// FactoryTypeTestFetcher corresponds to factory of test.TestFetcher
	FactoryTypeTestFetcher

	// MaxFactoryType defines the amount of supported factory types
	MaxFactoryType = iota - 1
)

func (factoryType FactoryType) String() string {
	switch factoryType {
	case FactoryTypeUndefined:
		return "undefined"
	case FactoryTypeTargetLocker:
		return "target.Locker"
	case FactoryTypeTargetManager:
		return "target.Manager"
	case FactoryTypeJobReporter:
		return "job.Reporter"
	case FactoryTypeTestStep:
		return "test.TestStep"
	case FactoryTypeTestFetcher:
		return "test.TestFetcher"
	}
	return "unknown"
}

// PluginRegistry manages all the plugins available in the system. It associates Plugin
// identifiers (implemented as simple strings) with factory functions that create instances
// of those plugins. A Plugin instance is owner by a single Job object.
type PluginRegistry struct {
	lock sync.RWMutex

	// FactoriesMap is the storage of all known factories (provided by plugins)
	// to be used to construct objects of required types.
	//
	// Key #0 (FactoryType) here is the type of the factory.
	// Key #1 (string) is the name (UniqueImplementationName) of the plugin.
	FactoriesMap [MaxFactoryType + 1]map[string]abstract.Factory
}

// NewPluginRegistry constructs a new empty plugin registry
func NewPluginRegistry() *PluginRegistry {
	pr := PluginRegistry{}
	for factoryType := FactoryType(0); factoryType <= MaxFactoryType; factoryType++ {
		pr.FactoriesMap[factoryType] = map[string]abstract.Factory{}
	}
	return &pr
}

func factoryTypeOf(factory abstract.Factory) FactoryType {
	switch factory.(type) {
	case target.LockerFactory:
		return FactoryTypeTargetLocker
	case target.TargetManagerFactory:
		return FactoryTypeTargetManager
	case job.ReporterFactory:
		return FactoryTypeJobReporter
	case test.TestStepFactory:
		return FactoryTypeTestStep
	case test.TestFetcherFactory:
		return FactoryTypeTestFetcher
	}
	return FactoryTypeUndefined
}

// RegisterFactory register the factory `factory` in the registry, so
// it will be retrievable via methods `Factory` and `Factories` and callable
// via methods `NewTestStep`, `NewTestFetcher`, `NewTargetManager` etc.
func (r *PluginRegistry) RegisterFactory(factory abstract.Factory) error {
	factoryType := factoryTypeOf(factory)
	if factoryType == FactoryTypeUndefined {
		return ErrUnknownFactoryType{Factory: factory}
	}

	// Checks

	factoryName := strings.ToLower(factory.UniqueImplementationName())
	if factoryName == `contest` {
		return ErrPluginNameIsReserved{factory}
	}
	if err := validateFactory(factory); err != nil { // factory-type specific checks
		return ErrValidationFailed{Err: err}
	}

	// Register the factory

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.FactoriesMap[factoryType][factoryName] != nil {
		return ErrDuplicateFactoryName{FactoryName: factoryName}
	}

	log.Infof("Registering factory '%T' with name '%s'", factory, factoryName)
	r.FactoriesMap[factoryType][factoryName] = factory
	return nil
}

// RegisterFactories does the same as RegisterFactoriesAsType, but it detects
// the type of each factory automatically. Factories `factories` are allowed to be
// of different types.
func (r *PluginRegistry) RegisterFactories(factories abstract.Factories) error {
	for _, factory := range factories {
		err := r.RegisterFactory(factory)
		if err != nil {
			return ErrUnableToRegisterFactory{Factory: factory, Err: err}
		}
	}
	return nil
}

// validateFactory performs checks specific to a factory type
//
// For example if factory has method `Events()` then events should be validated.
func validateFactory(factory abstract.Factory) error {
	switch factory := factory.(type) {
	case abstract.FactoryWithEvents:
		return event.Names(factory.Events()).Validate()
	}

	return nil
}

// Factories returns all factories of type defined by `factoryType`.
//
// It returns an error (ErrFactoryTypeNotFound) if no factories of type
// `factoryType` were registered.
func (r *PluginRegistry) Factories(
	factoryType FactoryType,
) (factories abstract.Factories, err error) {
	if factoryType <= FactoryTypeUndefined || factoryType > MaxFactoryType {
		return nil, ErrFactoryTypeNotFound{FactoryType: factoryType}
	}

	r.lock.RLock()
	defer r.lock.RUnlock()
	for _, factory := range r.FactoriesMap[factoryType] {
		factories = append(factories, factory)
	}
	return
}

// Factory returns the factory of type defined by `factoryType` and
// implementation name `factoryName`.
//
// `factoryName` is defined by method `UniqueImplementationName` of the factory.
//
// If the factory is not found then an error is returned.
func (r *PluginRegistry) Factory(
	factoryType FactoryType,
	factoryName string,
) (factory abstract.Factory, err error) {
	if factoryType <= FactoryTypeUndefined || factoryType > MaxFactoryType {
		return nil, ErrFactoryTypeNotFound{FactoryType: factoryType}
	}
	factoryName = strings.ToLower(factoryName)

	r.lock.RLock()
	defer r.lock.RUnlock()
	factory = r.FactoriesMap[factoryType][factoryName]

	if factory == nil {
		return nil, ErrFactoryNotFoundByName{ProductType: factoryType, FactoryName: factoryName}
	}

	if ft := factoryTypeOf(factory); ft != factoryType {
		return nil, ErrValidationFailed{Err: fmt.Errorf("invalid factory type: %s != %s", ft, factoryType)}
	}

	return
}

// NewTestStep calls the factory of the test.TestStep with name `pluginName`.
//
// `testStep` is the result of `test.TestStepFactory` method `New`.
// `allowedEventNames` is the result of `test.TestStepFactory` method `Events`
// converted to a map.
func (r *PluginRegistry) NewTestStep(pluginName string) (
	testStep test.TestStep,
	allowedEventNames map[event.Name]struct{},
	err error,
) {
	factoryAbstract, err := r.Factory(FactoryTypeTestStep, pluginName)
	if err != nil {
		return nil, nil, err
	}
	factory := factoryAbstract.(test.TestStepFactory)

	eventNames := event.Names(factory.Events())
	if err := eventNames.Validate(); err != nil {
		return nil, nil, err
	}

	return factory.New(), event.Names(factory.Events()).ToMap(), nil
}

// NewTestFetcher constructs a test.TestFetcher with implementation/plugin name `pluginName`.
func (r *PluginRegistry) NewTestFetcher(pluginName string) (test.TestFetcher, error) {
	factory, err := r.Factory(FactoryTypeTestFetcher, pluginName)
	if err != nil {
		return nil, fmt.Errorf("unable to get a test fetcher factory with name %s: %w", pluginName, err)
	}
	return factory.(test.TestFetcherFactory).New(), nil
}

// NewTargetManager constructs a target.TargetManager with implementation/plugin name `pluginName`.
func (r *PluginRegistry) NewTargetManager(pluginName string) (target.TargetManager, error) {
	factory, err := r.Factory(FactoryTypeTargetManager, pluginName)
	if err != nil {
		return nil, fmt.Errorf("unable to get a target manager factory with name %s: %w", pluginName, err)
	}
	return factory.(target.TargetManagerFactory).New(), nil
}

// NewReporter constructs a job.Reporter with implementation/plugin name `pluginName`.
func (r *PluginRegistry) NewReporter(pluginName string) (job.Reporter, error) {
	factory, err := r.Factory(FactoryTypeJobReporter, pluginName)
	if err != nil {
		return nil, fmt.Errorf("unable to get a job reporter factory with name %s: %w", pluginName, err)
	}
	return factory.(job.ReporterFactory).New(), nil
}

// NewTargetLocker constructs a target.Locker with implementation/plugin name `pluginName`.
//
// Arguments `timeout` and `arg` are passed to method `New` of target.LockerFactory.
func (r *PluginRegistry) NewTargetLocker(pluginName string, timeout time.Duration, arg string) (target.Locker, error) {
	factory, err := r.Factory(FactoryTypeTargetLocker, pluginName)
	if err != nil {
		return nil, fmt.Errorf("unable to get a target locker factory with name %s: %w", pluginName, err)
	}
	return factory.(target.LockerFactory).New(timeout, arg)
}
