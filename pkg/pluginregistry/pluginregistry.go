// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package pluginregistry

import (
	"fmt"
	"reflect"
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

// PluginRegistry manages all the plugins available in the system. It associates Plugin
// identifiers (implemented as simple strings) with factory functions that create instances
// of those plugins. A Plugin instance is owner by a single Job object.
type PluginRegistry struct {
	lock sync.RWMutex

	// FactoriesMap is the storage of all known factories (provided by plugins)
	// to be used to construct objects of required types.
	//
	// Key #0 (reflect.Type) here is the type of the factory (an interface it implements).
	// Key #1 (string) is the name (UniqueImplementationName) of the plugin.
	FactoriesMap map[reflect.Type]map[string]abstract.Factory
}

// NewPluginRegistry constructs a new empty plugin registry
func NewPluginRegistry() *PluginRegistry {
	pr := PluginRegistry{}
	pr.FactoriesMap = make(map[reflect.Type]map[string]abstract.Factory)
	return &pr
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

// RegisterFactoriesAsType performs RegisterFactoryAsType for each factory
// of `factories`.
func (r *PluginRegistry) RegisterFactoriesAsType(
	factoryTypePtr interface{},
	factories abstract.Factories,
) error {
	for _, factory := range factories {
		err := r.RegisterFactoryAsType(factoryTypePtr, factory)
		if err != nil {
			return ErrUnableToRegisterFactory{Factory: factory, Err: err}
		}
	}
	return nil
}

// RegisterFactory does the same as RegisterFactoryAsType, but it detects
// the type of the factory automatically.
func (r *PluginRegistry) RegisterFactory(factory abstract.Factory) error {
	var factoryTypePtr interface{}
	switch factory := factory.(type) {
	case target.LockerFactory:
		factoryTypePtr = (*target.LockerFactory)(nil)
	case target.TargetManagerFactory:
		factoryTypePtr = (*target.TargetManagerFactory)(nil)
	case job.ReporterFactory:
		factoryTypePtr = (*job.ReporterFactory)(nil)
	case test.TestStepFactory:
		factoryTypePtr = (*test.TestStepFactory)(nil)
	case test.TestFetcherFactory:
		factoryTypePtr = (*test.TestFetcherFactory)(nil)
	default:
		return ErrUnknownFactoryType{factory}
	}
	return r.registerFactoryAsType(reflect.TypeOf(factoryTypePtr).Elem(), factory)
}

// RegisterFactoryAsType registers the factory `factory` in the registry,
// so it will become accessible through methods `Factories`, `Factory`, `New*`.
//
// `factoryTypePtr` should be equal to a pointer to an interfaces which defines as
// which type the factory should be registered. Factory should implement this interface.
func (r *PluginRegistry) RegisterFactoryAsType(factoryTypePtr interface{}, factory abstract.Factory) error {
	if factoryTypePtr == nil {
		return ErrInvalidFactoryType{Description: "an untyped nil value is given"}
	}

	return r.registerFactoryAsType(reflect.TypeOf(factoryTypePtr).Elem(), factory)
}

func validateFactory(factory abstract.Factory) error {
	switch factory := factory.(type) {
	case abstract.FactoryWithEvents:
		return event.Names(factory.Events()).Validate()
	}
	return nil
}

func (r *PluginRegistry) registerFactoryAsType(factoryType reflect.Type, factory abstract.Factory) error {

	// Sanity checks

	if factory == nil {
		return ErrNilFactory{}
	}

	// Implementation names are case-insensitive
	// ITS: https://github.com/facebookincubator/contest/issues/14
	factoryName := strings.ToLower(factory.UniqueImplementationName())

	if factoryName == `contest` {
		// ITS: https://github.com/facebookincubator/contest/issues/10
		return ErrPluginNameIsReserved{factory}
	}

	// Check if `factoryType` at least implements `abstract.Factory`.
	abstractFactoryType := reflect.TypeOf((*abstract.Factory)(nil)).Elem()
	if !factoryType.Implements(abstractFactoryType) {
		return ErrInvalidFactoryType{FactoryType: factoryType, Description: "it does not implement abstract.Factory"}
	}
	// Check if `factory` really implements `factoryType`.
	if !reflect.TypeOf(factory).Implements(factoryType) {
		return ErrInvalidFactoryType{FactoryType: factoryType, Description: fmt.Sprintf("factory %T does not implement it", factory)}
	}

	// Factory-type specific checks
	if err := validateFactory(factory); err != nil {
		return ErrValidationFailed{Err: err}
	}

	// Register the factory

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.FactoriesMap[factoryType] == nil {
		r.FactoriesMap[factoryType] = map[string]abstract.Factory{}
	}

	if r.FactoriesMap[factoryType][factoryName] != nil {
		return ErrDuplicateFactoryName{FactoryName: factoryName}
	}

	log.Infof("Registering factory '%v' with name '%s' (%T)", factoryType.Name(), factoryName, factory)
	r.FactoriesMap[factoryType][factoryName] = factory
	return nil
}

// Factories returns all factories of type defined by `factoryTypePtr`.
//
// See description of `factoryTypePtr` within description
// of `RegisterFactoryAsType`.
//
// It returns an error (ErrFactoryTypeNotFound) if no factories of type
// `factoryTypePtr` were registered.
func (r *PluginRegistry) Factories(
	factoryTypePtr interface{},
) (factories abstract.Factories, err error) {
	factoryType := reflect.TypeOf(factoryTypePtr).Elem()

	r.lock.RLock()
	defer r.lock.RUnlock()

	if r.FactoriesMap[factoryType] == nil {
		return nil, ErrFactoryTypeNotFound{FactoryType: factoryType}
	}

	for _, factory := range r.FactoriesMap[factoryType] {
		factories = append(factories, factory)
	}
	return
}

// Factory returns the factory of type defined by `factoryTypePtr` and
// implementation name `factoryName`.
//
// See description of `factoryTypePtr` within description
// of `RegisterFactoryAsType`.
//
// `factoryName` is defined by method `UniqueImplementationName` of the factory.
//
// If the factory is not found then an error is returned.
func (r *PluginRegistry) Factory(
	factoryTypePtr interface{},
	factoryName string,
) (factory abstract.Factory, err error) {
	factoryName = strings.ToLower(factoryName)

	r.lock.RLock()
	defer r.lock.RUnlock()

	factoryType := reflect.TypeOf(factoryTypePtr).Elem()
	if r.FactoriesMap[factoryType] == nil {
		return nil, ErrFactoryTypeNotFound{FactoryType: factoryType}
	}
	if r.FactoriesMap[factoryType][factoryName] == nil {
		return nil, ErrFactoryNotFoundByName{ProductType: factoryType, FactoryName: factoryName}
	}

	return r.FactoriesMap[factoryType][factoryName], nil
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
	factoryAbstract, err := r.Factory((*test.TestStepFactory)(nil), pluginName)
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

// NewTestFetcher calls the factory of the test.TestFetcher with name `pluginName`.
func (r *PluginRegistry) NewTestFetcher(pluginName string) (test.TestFetcher, error) {
	factory, err := r.Factory((*test.TestFetcherFactory)(nil), pluginName)
	if err != nil {
		return nil, err
	}
	return factory.(test.TestFetcherFactory).New(), nil
}

// NewTargetManager calls the factory of the target.TargetManager with name `pluginName`.
func (r *PluginRegistry) NewTargetManager(pluginName string) (target.TargetManager, error) {
	factory, err := r.Factory((*target.TargetManagerFactory)(nil), pluginName)
	if err != nil {
		return nil, err
	}
	return factory.(target.TargetManagerFactory).New(), nil
}

// NewReporter calls the factory of the job.Reporter with name `pluginName`.
func (r *PluginRegistry) NewReporter(pluginName string) (job.Reporter, error) {
	factory, err := r.Factory((*job.ReporterFactory)(nil), pluginName)
	if err != nil {
		return nil, err
	}
	return factory.(job.ReporterFactory).New(), nil
}

// NewTargetLocker calls the factory of the target.Locker with name `pluginName`.
//
// Arguments `timeout` and `arg` are passed to method `New` of target.LockerFactory.
func (r *PluginRegistry) NewTargetLocker(pluginName string, timeout time.Duration, arg string) (target.Locker, error) {
	factory, err := r.Factory((*target.LockerFactory)(nil), pluginName)
	if err != nil {
		return nil, err
	}
	return factory.(target.LockerFactory).New(timeout, arg)
}
