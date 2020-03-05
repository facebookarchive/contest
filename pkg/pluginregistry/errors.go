package pluginregistry

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/facebookincubator/contest/pkg/abstract"
)

// ErrInvalidFactoryType means the first argument to RegisterFactoryAsType or
// RegisterFactoriesAsType was invalid.
type ErrInvalidFactoryType struct {
	FactoryType reflect.Type
	Description string
}

func (err ErrInvalidFactoryType) Error() string {
	return fmt.Sprintf("invalid factory type: %v (%s)", err.FactoryType.Name(), err.Description)
}

// ErrDuplicateFactoryName means it were found at least two factories with the same
// type and name.
type ErrDuplicateFactoryName struct {
	FactoryName string
}

func (err ErrDuplicateFactoryName) Error() string {
	return fmt.Sprintf("duplicate factory name: %s", err.FactoryName)
}

// ErrFactoryTypeNotFound means there was no factories requested of the selected
// type (were never registered).
type ErrFactoryTypeNotFound struct {
	FactoryType reflect.Type
}

func (err ErrFactoryTypeNotFound) Error() string {
	return fmt.Sprintf("factories of type %v not found", err.FactoryType)
}

// ErrFactoryNotFoundByName means there the factory with selected type and name
// was not found (was never registered).
type ErrFactoryNotFoundByName struct {
	ProductType reflect.Type
	FactoryName string
}

func (err ErrFactoryNotFoundByName) Error() string {
	return fmt.Sprintf("unable to find factory with name '%s' for product type '%v'", err.FactoryName, err.ProductType)
}

// ErrNilFactory means at least one of the passed factories has a nil-value.
type ErrNilFactory struct{}

func (err ErrNilFactory) Error() string { return "a nil-value factory" }

// ErrUnknownFactoryType means there was passed a factory of unknown type.
// Most likely there was added a new type of factories recently and it
// wasn't added to function `RegisterFactory`.
type ErrUnknownFactoryType struct {
	Factory abstract.Factory
}

func (err ErrUnknownFactoryType) Error() string {
	return fmt.Sprintf("cannot determine factory type of %T", err.Factory)
}

// ErrPluginNameIsReserved there was an attempt to register a plugin with
// a reserved name.
//
// See: https://github.com/facebookincubator/contest/issues/10
type ErrPluginNameIsReserved struct {
	Factory abstract.Factory
}

func (err ErrPluginNameIsReserved) Error() string {
	return fmt.Sprintf("plugin name '%s' is reserved, cannot add factory %T",
		strings.ToLower(err.Factory.UniqueImplementationName()),
		err.Factory)
}

// ErrValidationFailed means the factory failed validation checks specific
// to its type.
type ErrValidationFailed struct {
	Err error
}

func (err ErrValidationFailed) Error() string {
	return fmt.Sprintf("validation failed: %v", err.Err)
}
func (err ErrValidationFailed) Unwrap() error {
	return err.Err
}

// ErrUnableToRegisterFactory the factory cannot be registered.
// The reason is described by `Err`.
type ErrUnableToRegisterFactory struct {
	Factory abstract.Factory
	Err     error
}

func (err ErrUnableToRegisterFactory) Error() string {
	return fmt.Sprintf("unable to register factory '%s' (%T): %v",
		err.Factory.UniqueImplementationName(), err.Factory, err.Err)
}
func (err ErrUnableToRegisterFactory) Unwrap() error {
	return err.Err
}
