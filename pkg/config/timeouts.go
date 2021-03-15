// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package config

import "time"

// TargetManagerAcquireTimeout represents the maximum time that JobManager should wait
// for the execution of Acquire function from the chosen TargetManager
var TargetManagerAcquireTimeout = 5 * time.Minute

// TargetManagerReleaseTimeout represents the maximum time that JobManager should wait
// for the execution of Release function from the chosen TargetManager
var TargetManagerReleaseTimeout = 5 * time.Minute

// TestRunnerMsgTimeout represents the maximum time that any component of the
// TestRunner will wait for the delivery of a message to any other subsystem
// of the TestRunner
var TestRunnerMsgTimeout = 5 * time.Second

// TestRunnerShutdownTimeout represents the maximum time that the TestRunner will
// wait for all the TestStep to complete after a cancellation signal has been
// delivered

// TestRunnerShutdownTimeout controls a block of the TestRunner which works as a
// watchdog, i.e. if there are multiple steps that need to return, the timeout is
// reset every time a step returns. The timeout should be handled so that it
// doesn't reset when a TestStep returns.
var TestRunnerShutdownTimeout = 30 * time.Second

// TestRunnerStepShutdownTimeout represents the maximum time that the TestRunner
// will wait for all TestSteps to complete after all Targets have reached the end
// of the pipeline. This timeout is only relevant if a cancellation signal is *not*
// delivered.

// TestRunnerStepShutdownTimeout controls a block of the TestRunner which worksas
// a watchdog, i.e. if there are multiple steps that need to return, the timeout
// is reset every time a step returns. The timeout should be handled so that it
// doesn't reset when a TestStep returns.
var TestRunnerStepShutdownTimeout = 5 * time.Second

// LockRefreshTimeout is the amount of time by which a target lock is extended
// periodically while a job is running.
var LockRefreshTimeout = 1 * time.Minute
