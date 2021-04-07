// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package jobmanager

import (
	"time"

	"github.com/benbjohnson/clock"

	"github.com/facebookincubator/contest/pkg/api"
	configPkg "github.com/facebookincubator/contest/pkg/config"
)

// Option is an additional argument to method New to change the behavior
// of the JobManager.
type Option interface {
	apply(*config)
}

type config struct {
	apiOptions         []api.Option
	instanceTag        string
	targetLockDuration time.Duration
	clock              clock.Clock
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

// OptionInstanceTag wraps a string to be used as instance tag.
type OptionInstanceTag string

func (opt OptionInstanceTag) apply(config *config) {
	config.instanceTag = string(opt)
}

// OptionTargetLockDuration wraps a string to be used as instance tag.
type OptionTargetLockDuration time.Duration

func (opt OptionTargetLockDuration) apply(config *config) {
	config.targetLockDuration = time.Duration(opt)
}

// OptionClock wraps a string to be used as instance tag.
type optionClock struct {
	clock clock.Clock
}

func (opt optionClock) apply(config *config) {
	config.clock = opt.clock
}

func OptionClock(clk clock.Clock) Option {
	return optionClock{clock: clk}
}

// getConfig converts a set of Option-s into one structure "Config".
func getConfig(opts ...Option) config {
	result := config{
		targetLockDuration: configPkg.DefaultTargetLockDuration,
		clock:              clock.New(),
	}
	for _, opt := range opts {
		opt.apply(&result)
	}
	return result
}
