// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import "encoding/json"

// Reporting is the configuration section that determines how to report the
// results. It is divided in run reporting and final reporting. The run
// reporters are called when each test run is completed, while the final
// reporters are called once at the end of the whole set of runs is completed.
// The final reporters are optional.
// If the job is continuous (e.g. run indefinitely), the final reporters are
// only called if the job is interrupted.
type Reporting struct {
	RunReporters   []ReporterConfig
	FinalReporters []ReporterConfig
}

// ReporterConfig is the configuration of a specific reporter, either a run
// reporter or a final reporter.
type ReporterConfig struct {
	Name       string
	Parameters json.RawMessage
}
