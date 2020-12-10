// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package config

import "time"

// TargetManagerTimeout represents the maximum time that JobManager should wait
// for the execution of Acquire and Release functions from the chosen TargetManager
var TargetManagerTimeout = 5 * time.Minute

// StepInjectTimeout represents the maximum time that TestRunner will wait for
// a TestStep to accept a Target
var StepInjectTimeout = 30 * time.Second

// TestRunnerShutdownTimeout represents the maximum time that the TestRunner
// will wait for all TestSteps to complete after all Targets have reached the end
// of the pipeline.
var TestRunnerShutdownTimeout = 30 * time.Second

// LockRefreshTimeout is the amount of time by which a target lock is extended
// periodically while a job is running.
var LockRefreshTimeout = 1 * time.Minute

// LockInitialTimeout is the initial lock duration when acquiring a new lock
// during target acquisition. This should include TargetManagerTimeout to
// allow for dynamic locking in the target manager.
var LockInitialTimeout = TargetManagerTimeout + LockRefreshTimeout
