// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"github.com/facebookincubator/contest/pkg/api"
)

// Option is an additional argument to method New to change the behavior
// of the JobManager.
type Option interface {
	apply(*config)
}

type config struct {
	apiOptions []api.Option
}

// OptionAPI wraps api.Option to implement Option.
type OptionAPI struct {
	api.Option
}

// apply implements Option.
func (opt OptionAPI) apply(config *config) {
	config.apiOptions = append(config.apiOptions, opt.Option)
}

// APIOption is a syntax-sugar function which just wraps an api.Option
// into OptionAPI.
func APIOption(option api.Option) Option {
	return OptionAPI{Option: option}
}

// getConfig converts a set of Option-s into one structure "Config".
func getConfig(opts ...Option) config {
	result := config{}
	for _, opt := range opts {
		opt.apply(&result)
	}
	return result
}
