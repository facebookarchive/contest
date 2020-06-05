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
// the first Step of the pipeline to accept a Target
var StepInjectTimeout = 30 * time.Second

// TestRunnerMsgTimeout represents the maximum time that any component of the
// TestRunner will wait for the delivery of a message to any other subsystem
// of the TestRunner
var TestRunnerMsgTimeout = 5 * time.Second

// TestRunnerShutdownTimeout represents the maximum time that the TestRunner will
// wait for all the Step to complete after a cancellation signal has been
// delivered

// TestRunnerShutdownTimeout controls a block of the TestRunner which works as a
// watchdog, i.e. if there are multiple steps that need to return, the timeout is
// reset every time a step returns. The timeout should be handled so that it
// doesn't reset when a Step returns.
var TestRunnerShutdownTimeout = 30 * time.Second

// TestRunnerStepShutdownTimeout represents the maximum time that the TestRunner
// will wait for all Steps to complete after all Targets have reached the end
// of the pipeline. This timeout is only relevant if a cancellation signal is *not*
// delivered.

// TestRunnerStepShutdownTimeout controls a block of the TestRunner which worksas
// a watchdog, i.e. if there are multiple steps that need to return, the timeout
// is reset every time a step returns. The timeout should be handled so that it
// doesn't reset when a Step returns.
var TestRunnerStepShutdownTimeout = 5 * time.Second

// LockTimeout represent the amount of time that a lock is held for a target
var LockTimeout = 1 * time.Minute
