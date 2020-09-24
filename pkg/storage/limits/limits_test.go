// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package limits_test

import (
	"encoding/json"
	"errors"
	"strings"

	"testing"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage/limits"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/plugins/targetmanagers/targetlist"
	"github.com/facebookincubator/contest/plugins/testfetchers/literal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This tests are bad, because they touche so may things which are not related to storage limitations and
// depend on order of checks in validation code, but this is the price of having them all in one package

func TestServerIDPanics(t *testing.T) {
	apiInst := api.New(func() string { return strings.Repeat("A", limits.MaxServerIDLen+1) })
	assert.Panics(t, func() { apiInst.ServerID() })
}

type eventMsg struct {
	requestor api.EventRequestor
}

func (e eventMsg) Requestor() api.EventRequestor { return e.requestor }
func TestRequestorName(t *testing.T) {
	apiInst := api.API{}
	timeout := time.Second
	err := apiInst.SendEvent(&api.Event{
		Msg: eventMsg{requestor: api.EventRequestor(strings.Repeat("A", limits.MaxRequestorNameLen+1))},
	}, &timeout)
	assertLenError(t, "Requestor name", err)
}

func TestEventName(t *testing.T) {
	eventName := event.Name(strings.Repeat("A", limits.MaxEventNameLen+1))
	err := eventName.Validate()
	assertLenError(t, "Event name", err)
}

func TestJobName(t *testing.T) {
	jd := job.JobDescriptor{TestDescriptors: []*test.TestDescriptor{{}}, JobName: strings.Repeat("A", limits.MaxJobNameLen+1)}
	jsonJd, err := json.Marshal(&jd)
	require.NoError(t, err)
	_, err = jobmanager.NewJobFromRequest(&pluginregistry.PluginRegistry{}, &job.Request{JobDescriptor: string(jsonJd)})
	assertLenError(t, "Job name", err)
}

func TestReporterName(t *testing.T) {
	jd := job.JobDescriptor{
		TestDescriptors: []*test.TestDescriptor{{}},
		JobName:         "AA",
		Reporting:       job.Reporting{RunReporters: []job.ReporterConfig{{Name: strings.Repeat("A", limits.MaxReporterNameLen+1)}}},
	}
	jsonJd, err := json.Marshal(&jd)
	require.NoError(t, err)
	_, err = jobmanager.NewJobFromRequest(&pluginregistry.PluginRegistry{},
		&job.Request{JobDescriptor: string(jsonJd)},
	)
	assertLenError(t, "Reporter name", err)
}
func TestTestName(t *testing.T) {
	pluginRegistry := pluginregistry.NewPluginRegistry()
	err := pluginRegistry.RegisterTargetManager(targetlist.Load())
	require.NoError(t, err)
	err = pluginRegistry.RegisterTestFetcher(literal.Load())
	require.NoError(t, err)

	testFetchParams, err := json.Marshal(&literal.FetchParameters{
		TestName: strings.Repeat("A", limits.MaxTestNameLen+1),
	})
	require.NoError(t, err)

	jd := job.JobDescriptor{
		TestDescriptors: []*test.TestDescriptor{{
			TargetManagerName:          "targetList",
			TestFetcherName:            "literal",
			TestFetcherFetchParameters: testFetchParams,
		}},
		JobName:   "AA",
		Reporting: job.Reporting{RunReporters: []job.ReporterConfig{{Name: "BB"}}},
	}
	jsonJd, err := json.Marshal(&jd)
	require.NoError(t, err)
	_, err = jobmanager.NewJobFromRequest(pluginRegistry,
		&job.Request{JobDescriptor: string(jsonJd)},
	)
	assertLenError(t, "Test name", err)
}

func TestTestStepLabel(t *testing.T) {
	pluginRegistry := pluginregistry.NewPluginRegistry()
	err := pluginRegistry.RegisterTargetManager(targetlist.Load())
	require.NoError(t, err)
	err = pluginRegistry.RegisterTestFetcher(literal.Load())
	require.NoError(t, err)

	testFetchParams, err := json.Marshal(&literal.FetchParameters{
		TestName: "AA",
		Steps: []*test.TestStepDescriptor{{
			Label: strings.Repeat("A", limits.MaxTestStepLabelLen+1),
		}},
	})
	require.NoError(t, err)

	jd := job.JobDescriptor{
		TestDescriptors: []*test.TestDescriptor{{
			TargetManagerName:          "targetList",
			TestFetcherName:            "literal",
			TestFetcherFetchParameters: testFetchParams,
		}},
		JobName:   "AA",
		Reporting: job.Reporting{RunReporters: []job.ReporterConfig{{Name: "BB"}}},
	}
	jsonJd, err := json.Marshal(&jd)
	require.NoError(t, err)
	_, err = jobmanager.NewJobFromRequest(pluginRegistry,
		&job.Request{JobDescriptor: string(jsonJd)},
	)
	assertLenError(t, "Test step label", err)
}

func assertLenError(t *testing.T, name string, err error) {
	var lenErr limits.ErrParameterIsTooLong
	require.Truef(t, errors.As(err, &lenErr), "got %v instead of ErrParameterIsTooLong", err)
	assert.Equal(t, name, lenErr.DataName)
}
