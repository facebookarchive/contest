// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package api

import (
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
)

// ResponseType defines the storage type of a response type.
type ResponseType int32

// The various response types used in the Response struct.
const (
	ResponseTypeStart ResponseType = iota
	ResponseTypeStop
	ResponseTypeStatus
	ResponseTypeRetry
	ResponseTypeVersion
)

// ResponseTypeToName maps response types to their names.
var ResponseTypeToName = map[ResponseType]string{
	ResponseTypeStart:   "ResponseTypeStart",
	ResponseTypeStop:    "ResponseTypeStop",
	ResponseTypeStatus:  "ResponseTypeStatus",
	ResponseTypeRetry:   "ResponseTypeRetry",
	ResponseTypeVersion: "ResponseTypeVersion",
}

// Response is the type returned to any API request.
type Response struct {
	ServerID string
	Type     ResponseType
	Data     ResponseData
	Err      error
}

// ResponseData is the interface type implemented by the various response types.
type ResponseData interface {
	Type() ResponseType
}

// ResponseDataStart is the response type for a Start request.
type ResponseDataStart struct {
	JobID types.JobID
}

// Type returns the response type.
func (r ResponseDataStart) Type() ResponseType {
	return ResponseTypeStart
}

// ResponseDataStop is the response type for a Stop request.
type ResponseDataStop struct {
}

// Type returns the response type.
func (r ResponseDataStop) Type() ResponseType {
	return ResponseTypeStop
}

// ResponseDataStatus is the response type for a Status request.
type ResponseDataStatus struct {
	Status *job.Status
}

// Type returns the response type.
func (r ResponseDataStatus) Type() ResponseType {
	return ResponseTypeStatus
}

// ResponseDataRetry is the response type for a Retry request.
type ResponseDataRetry struct {
	JobID    types.JobID
	NewJobID types.JobID
}

// Type returns the response type.
func (r ResponseDataRetry) Type() ResponseType {
	return ResponseTypeRetry
}

// ResponseDataVersion is the response type for a Version request.
type ResponseDataVersion struct {
	Version uint32
}

// Type returns the response type.
func (r ResponseDataVersion) Type() ResponseType {
	return ResponseTypeVersion
}
