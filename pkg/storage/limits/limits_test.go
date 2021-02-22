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
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/plugins/reporters/noop"
	"github.com/facebookincubator/contest/plugins/targetmanagers/targetlist"
	"github.com/facebookincubator/contest/plugins/testfetchers/literal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This tests are bad, because they touche so may things which are not related to storage limitations and
// depend on order of checks in validation code, but this is the price of having them all in one package

var (
	ctx = logrusctx.NewContext(logger.LevelDebug)
)

func TestServerID(t *testing.T) {
	_, err := api.New(api.OptionServerID(strings.Repeat("A", limits.MaxServerIDLen+1)))
	assertLenError(t, "Server ID", err)
}

type eventMsg struct {
	requestor api.EventRequestor
}

func (e eventMsg) Requestor() api.EventRequestor { return e.requestor }
func TestRequestorName(t *testing.T) {
	apiInst := api.API{}
	timeout := time.Second
	err := apiInst.SendEvent(&api.Event{
		Context: xcontext.Background(),
		Msg:     eventMsg{requestor: api.EventRequestor(strings.Repeat("A", limits.MaxRequestorNameLen+1))},
	}, &timeout)
	assertLenError(t, "Requestor name", err)
}

func TestEventName(t *testing.T) {
	eventName := event.Name(strings.Repeat("A", limits.MaxEventNameLen+1))
	err := eventName.Validate()
	assertLenError(t, "Event name", err)
}

func TestJobName(t *testing.T) {
	jd := job.Descriptor{TestDescriptors: []*test.TestDescriptor{{}}, JobName: strings.Repeat("A", limits.MaxJobNameLen+1)}
	jsonJd, err := json.Marshal(&jd)
	require.NoError(t, err)
	_, err = jobmanager.NewJobFromJSONDescriptor(xcontext.Background(), &pluginregistry.PluginRegistry{}, string(jsonJd))
	assertLenError(t, "Job name", err)
}

func TestReporterName(t *testing.T) {
	jd := job.Descriptor{
		TestDescriptors: []*test.TestDescriptor{{}},
		JobName:         "AA",
		Reporting:       job.Reporting{RunReporters: []job.ReporterConfig{{Name: strings.Repeat("A", limits.MaxReporterNameLen+1)}}},
	}
	jsonJd, err := json.Marshal(&jd)
	require.NoError(t, err)
	_, err = jobmanager.NewJobFromJSONDescriptor(xcontext.Background(), &pluginregistry.PluginRegistry{}, string(jsonJd))
	assertLenError(t, "Reporter name", err)
}
func TestTestName(t *testing.T) {
	pluginRegistry := pluginregistry.NewPluginRegistry(ctx)
	err := pluginRegistry.RegisterTargetManager(targetlist.Load())
	require.NoError(t, err)
	err = pluginRegistry.RegisterTestFetcher(literal.Load())
	require.NoError(t, err)
	err = pluginRegistry.RegisterReporter(noop.Load())
	require.NoError(t, err)

	testFetchParams, err := json.Marshal(&literal.FetchParameters{
		TestName: strings.Repeat("A", limits.MaxTestNameLen+1),
	})
	require.NoError(t, err)

	jd := job.Descriptor{
		TestDescriptors: []*test.TestDescriptor{{
			TargetManagerName:          "targetList",
			TestFetcherName:            "literal",
			TestFetcherFetchParameters: testFetchParams,
		}},
		JobName:   "AA",
		Reporting: job.Reporting{RunReporters: []job.ReporterConfig{{Name: "noop"}}},
	}
	jsonJd, err := json.Marshal(&jd)
	require.NoError(t, err)
	_, err = jobmanager.NewJobFromJSONDescriptor(ctx, pluginRegistry, string(jsonJd))
	assertLenError(t, "Test name", err)
}

func TestTestStepLabel(t *testing.T) {
	pluginRegistry := pluginregistry.NewPluginRegistry(ctx)
	err := pluginRegistry.RegisterTargetManager(targetlist.Load())
	require.NoError(t, err)
	err = pluginRegistry.RegisterTestFetcher(literal.Load())
	require.NoError(t, err)
	err = pluginRegistry.RegisterReporter(noop.Load())
	require.NoError(t, err)

	testFetchParams, err := json.Marshal(&literal.FetchParameters{
		TestName: "AA",
		Steps: []*test.TestStepDescriptor{{
			Label: strings.Repeat("A", limits.MaxTestStepLabelLen+1),
		}},
	})
	require.NoError(t, err)

	jd := job.Descriptor{
		TestDescriptors: []*test.TestDescriptor{{
			TargetManagerName:          "targetList",
			TestFetcherName:            "literal",
			TestFetcherFetchParameters: testFetchParams,
		}},
		JobName:   "AA",
		Reporting: job.Reporting{RunReporters: []job.ReporterConfig{{Name: "noop"}}},
	}
	jsonJd, err := json.Marshal(&jd)
	require.NoError(t, err)
	_, err = jobmanager.NewJobFromJSONDescriptor(ctx, pluginRegistry, string(jsonJd))
	assertLenError(t, "Test step label", err)
}

func assertLenError(t *testing.T, name string, err error) {
	var lenErr limits.ErrParameterIsTooLong
	require.Truef(t, errors.As(err, &lenErr), "got %v instead of ErrParameterIsTooLong", err)
	assert.Equal(t, name, lenErr.DataName)
}
