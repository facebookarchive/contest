// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package buildinfo

var (
	// BuildMode is usually "prod" or "dev".
	BuildMode string

	// BuildDate is when the binary was built.
	BuildDate string

	// Revision is the commit.
	Revision string
)
