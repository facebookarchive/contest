// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package runner

import (
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/pkg/xcontext/bundles/logrusctx"
	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/tests/common"
	"github.com/facebookincubator/contest/tests/common/goroutine_leak_check"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/badtargets"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/channels"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/hanging"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/noreturn"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/panicstep"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/teststep"
)

const (
	testName        = "SimpleTest"
	shutdownTimeout = 3 * time.Second
)

var (
	evs            storage.ResettableStorage
	pluginRegistry *pluginregistry.PluginRegistry
)

var (
	ctx = logrusctx.NewContext(logger.LevelDebug)
)

func TestMain(m *testing.M) {
	flag.Parse()
	if ms, err := memory.New(); err == nil {
		evs = ms
		if err := storage.SetStorage(ms); err != nil {
			panic(err.Error())
		}
	} else {
		panic(fmt.Sprintf("could not initialize in-memory storage layer: %v", err))
	}
	pluginRegistry = pluginregistry.NewPluginRegistry(ctx)
	for _, e := range []struct {
		name    string
		factory test.TestStepFactory
		events  []event.Name
	}{
		{badtargets.Name, badtargets.New, badtargets.Events},
		{channels.Name, channels.New, channels.Events},
		{hanging.Name, hanging.New, hanging.Events},
		{noreturn.Name, noreturn.New, noreturn.Events},
		{panicstep.Name, panicstep.New, panicstep.Events},
		{teststep.Name, teststep.New, teststep.Events},
	} {
		if err := pluginRegistry.RegisterTestStep(e.name, e.factory, e.events); err != nil {
			panic(fmt.Sprintf("could not register TestStep: %v", err))
		}
	}
	flag.Parse()
	goroutine_leak_check.LeakCheckingTestMain(m,
		// We expect these to leak.
		"github.com/facebookincubator/contest/tests/plugins/teststeps/hanging.(*hanging).Run",
		"github.com/facebookincubator/contest/tests/plugins/teststeps/noreturn.(*noreturnStep).Run",

		// No leak in contexts checked with itsown unit-tests
		"github.com/facebookincubator/contest/pkg/xcontext.(*ctxValue).cloneWithStdContext.func2",
	)
}

func newTestRunner() *TestRunner {
	return NewTestRunnerWithTimeouts(shutdownTimeout)
}

func resetEventStorage() {
	if err := evs.Reset(); err != nil {
		panic(err.Error())
	}
}

func tgt(id string) *target.Target {
	return &target.Target{ID: id}
}

func getStepEvents(stepLabel string) string {
	return common.GetTestEventsAsString(ctx, evs, testName, nil, &stepLabel)
}

func getTargetEvents(targetID string) string {
	return common.GetTestEventsAsString(ctx, evs, testName, &targetID, nil)
}

func newStep(label, name string, params *test.TestStepParameters) test.TestStepBundle {
	td := test.TestStepDescriptor{
		Name:  name,
		Label: label,
	}
	if params != nil {
		td.Parameters = *params
	}
	sb, err := pluginRegistry.NewTestStepBundle(ctx, td, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create test step bundle: %v", err))
	}
	return *sb
}

func newTestStep(label string, failPct int, failTargets string, delayTargets string) test.TestStepBundle {
	return newStep(label, teststep.Name, &test.TestStepParameters{
		teststep.FailPctParam:      []test.Param{*test.NewParam(fmt.Sprintf("%d", failPct))},
		teststep.FailTargetsParam:  []test.Param{*test.NewParam(failTargets)},
		teststep.DelayTargetsParam: []test.Param{*test.NewParam(delayTargets)},
	})
}

type runRes struct {
	res []byte
	err error
}

func runWithTimeout(t *testing.T, tr *TestRunner, ctx xcontext.Context, resumeState []byte, runID types.RunID, timeout time.Duration, targets []*target.Target, bundles []test.TestStepBundle) ([]byte, error) {
	newCtx, cancel := xcontext.WithCancel(ctx)
	test := &test.Test{
		Name:             testName,
		TestStepsBundles: bundles,
	}
	resCh := make(chan runRes)
	go func() {
		res, err := tr.Run(newCtx, test, targets, 1, runID, resumeState)
		resCh <- runRes{res: res, err: err}
	}()
	var res runRes
	select {
	case res = <-resCh:
	case <-time.After(timeout):
		cancel()
		assert.FailNow(t, "TestRunner should not time out")
	}
	return res.res, res.err
}

// Simple case: one target, one step, success.
func Test1Step1Success(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newTestStep("Step 1", 0, "", ""),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents(""))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestFinishedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetOut]}
`, getTargetEvents("T1"))
}

// Simple case: one target, one step that blocks for a bit, success.
// We block for longer than the shutdown timeout of the test runner.
func Test1StepLongerThanShutdown1Success(t *testing.T) {
	resetEventStorage()
	tr := NewTestRunnerWithTimeouts(100 * time.Millisecond)
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newTestStep("Step 1", 0, "", "T1=500"),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents(""))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestFinishedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetOut]}
`, getTargetEvents("T1"))
}

// Simple case: one target, one step, failure.
func Test1Step1Fail(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newTestStep("Step 1", 100, "", ""),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents("Step 1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestFailedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetErr &"{\"Error\":\"target failed\"}"]}
`, getTargetEvents("T1"))
}

// One step pipeline with two targets - one fails, one succeeds.
func Test1Step1Success1Fail(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1"), tgt("T2")},
		[]test.TestStepBundle{
			newTestStep("Step 1", 0, "T1", "T2=100"),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents(""))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestFailedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetErr &"{\"Error\":\"target failed\"}"]}
`, getTargetEvents("T1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TestFinishedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TargetOut]}
`, getTargetEvents("T2"))
}

// Three-step pipeline, two targets: T1 fails at step 1, T2 fails at step 2,
// step 3 is not reached and not even run.
func Test3StepsNotReachedStepNotRun(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1"), tgt("T2")},
		[]test.TestStepBundle{
			newTestStep("Step 1", 0, "T1", ""),
			newTestStep("Step 2", 0, "T2", ""),
			newTestStep("Step 3", 0, "", ""),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents("Step 1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 2][(*Target)(nil) TestStepRunningEvent]}
{[1 1 SimpleTest Step 2][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents("Step 2"))
	require.Equal(t, "\n\n", getStepEvents("Step 3"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestFailedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetErr &"{\"Error\":\"target failed\"}"]}
`, getTargetEvents("T1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TestFinishedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TargetOut]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TargetIn]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TestStartedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TestFailedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TargetErr &"{\"Error\":\"target failed\"}"]}
`, getTargetEvents("T2"))
}

// A misbehaving step that fails to shut down properly after processing targets
// and does not return.
func TestNoReturnStepWithCorrectTargetForwarding(t *testing.T) {
	resetEventStorage()
	tr := NewTestRunnerWithTimeouts(200 * time.Millisecond)
	ctx, cancel := xcontext.WithCancel(ctx)
	defer cancel()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newStep("Step 1", noreturn.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepsNeverReturned{}, err)
}

// A misbehaving step that panics.
func TestStepPanics(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newStep("Step 1", panicstep.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepPaniced{}, err)
	require.Equal(t, "\n\n", getTargetEvents("T1"))
	require.Contains(t, getStepEvents("Step 1"), "step Step 1 paniced")
}

// A misbehaving step that closes its output channel.
func TestStepClosesChannels(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newStep("Step 1", channels.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepClosedChannels{}, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetOut]}
`, getTargetEvents("T1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) TestError &"\"test step Step 1 closed output channels (api violation)\""]}
`, getStepEvents("Step 1"))
}

// A misbehaving step that yields a result for a target that does not exist.
func TestStepYieldsResultForNonexistentTarget(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("TExtra")},
		[]test.TestStepBundle{
			newStep("Step 1", badtargets.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepReturnedUnexpectedResult{}, err)
	require.Equal(t, "\n\n", getTargetEvents("TExtra2"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) TestError &"\"test step Step 1 returned unexpected result for TExtra2\""]}
`, getStepEvents("Step 1"))
}

// A misbehaving step that yields a duplicate result for a target.
func TestStepYieldsDuplicateResult(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("TGood"), tgt("TDup")},
		[]test.TestStepBundle{
			// TGood makes it past here unscathed and gets delayed in Step 2,
			// TDup also emerges fine at first but is then returned again, and that's bad.
			newStep("Step 1", badtargets.Name, nil),
			newTestStep("Step 2", 0, "", "TGood=100"),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepReturnedDuplicateResult{}, err)
}

// A misbehaving step that loses targets.
func TestStepLosesTargets(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("TGood"), tgt("TDrop")},
		[]test.TestStepBundle{
			newStep("Step 1", badtargets.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepLostTargets{}, err)
	require.Contains(t, err.Error(), "TDrop")
}

// A misbehaving step that yields a result for a target that does exist
// but is not currently waiting for it.
func TestStepYieldsResultForUnexpectedTarget(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("TExtra"), tgt("TExtra2")},
		[]test.TestStepBundle{
			// TExtra2 fails here.
			newTestStep("Step 1", 0, "TExtra2", ""),
			// Yet, a result for it is returned here, which we did not expect.
			newStep("Step 2", badtargets.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepReturnedUnexpectedResult{}, err)
}

// Larger, randomized test - a number of steps, some targets failing, some succeeding.
func TestRandomizedMultiStep(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	var targets []*target.Target
	for i := 1; i <= 100; i++ {
		targets = append(targets, tgt(fmt.Sprintf("T%d", i)))
	}
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		targets,
		[]test.TestStepBundle{
			newTestStep("Step 1", 0, "", "*=10"),  // All targets pass the first step, with a slight delay
			newTestStep("Step 2", 25, "", ""),     // 25% don't make it past the second step
			newTestStep("Step 3", 25, "", "*=10"), // Another 25% fail at the third step
		},
	)
	require.NoError(t, err)
	// Every target mush have started and finished the first step.
	numFinished := 0
	for _, tgt := range targets {
		s1n := "Step 1"
		require.Equal(t, fmt.Sprintf(`
{[1 1 SimpleTest Step 1][Target{ID: "%s"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "%s"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "%s"} TestFinishedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "%s"} TargetOut]}
`, tgt.ID, tgt.ID, tgt.ID, tgt.ID),
			common.GetTestEventsAsString(ctx, evs, testName, &tgt.ID, &s1n))
		s3n := "Step 3"
		if strings.Contains(common.GetTestEventsAsString(ctx, evs, testName, &tgt.ID, &s3n), "TestFinishedEvent") {
			numFinished++
		}
	}
	// At least some must have finished.
	require.Greater(t, numFinished, 0)
}

// Test pausing/resuming a naive step that does not cooperate.
// In this case we drain input, wait for all targets to emerge and exit gracefully.
func TestPauseResumeSimple(t *testing.T) {
	resetEventStorage()
	var err error
	var resumeState []byte
	targets := []*target.Target{tgt("T1"), tgt("T2"), tgt("T3")}
	steps := []test.TestStepBundle{
		newTestStep("Step 1", 0, "T1", ""),
		// T2 and T3 will be paused here, the step will be given time to finish.
		newTestStep("Step 2", 0, "", "T2=200,T3=200"),
		newTestStep("Step 3", 0, "", ""),
	}
	{
		tr1 := newTestRunner()
		ctx1, pause := xcontext.WithNotify(ctx, xcontext.ErrPaused)
		ctx1, cancel := xcontext.WithCancel(ctx1)
		defer cancel()
		go func() {
			time.Sleep(100 * time.Millisecond)
			ctx.Infof("TestPauseResumeNaive: pausing")
			pause()
		}()
		resumeState, err = runWithTimeout(t, tr1, ctx1, nil, 1, 2*time.Second, targets, steps)
		require.Error(t, err)
		require.IsType(t, xcontext.ErrPaused, err)
		require.NotNil(t, resumeState)
	}
	ctx.Debugf("Resume state: %s", string(resumeState))
	// Make sure that resume state is validated.
	{
		tr := newTestRunner()
		ctx, cancel := xcontext.WithCancel(ctx)
		defer cancel()
		resumeState2, err := runWithTimeout(
			t, tr, ctx, []byte("FOO"), 2, 2*time.Second, targets, steps)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid resume state")
		require.Nil(t, resumeState2)
	}
	{
		tr := newTestRunner()
		ctx, cancel := xcontext.WithCancel(ctx)
		defer cancel()
		resumeState2 := strings.Replace(string(resumeState), `"v"`, `"Xv"`, 1)
		_, err := runWithTimeout(
			t, tr, ctx, []byte(resumeState2), 3, 2*time.Second, targets, steps)
		require.Error(t, err)
		require.Contains(t, err.Error(), "incompatible resume state")
	}
	{
		tr := newTestRunner()
		ctx, cancel := xcontext.WithCancel(ctx)
		defer cancel()
		resumeState2 := strings.Replace(string(resumeState), `"job_id":1`, `"job_id":2`, 1)
		_, err := runWithTimeout(
			t, tr, ctx, []byte(resumeState2), 4, 2*time.Second, targets, steps)
		require.Error(t, err)
		require.Contains(t, err.Error(), "wrong resume state")
	}
	// Finally, resume and finish the job.
	{
		tr2 := newTestRunner()
		ctx2, cancel := xcontext.WithCancel(ctx)
		defer cancel()
		_, err := runWithTimeout(t, tr2, ctx2, resumeState, 5, 2*time.Second,
			// Pass exactly the same targets and pipeline to resume properly.
			// Don't use the same pointers ot make sure there is no reliance on that.
			[]*target.Target{tgt("T1"), tgt("T2"), tgt("T3")},
			[]test.TestStepBundle{
				newTestStep("Step 1", 0, "T1", ""),
				newTestStep("Step 2", 0, "", "T2=200,T3=200"),
				newTestStep("Step 3", 0, "", ""),
			},
		)
		require.NoError(t, err)
	}
	// Verify step events.
	// Steps 1 and 2 are executed entirely within the first runner instance
	// and never started in the second.
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents("Step 1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 2][(*Target)(nil) TestStepRunningEvent]}
{[1 1 SimpleTest Step 2][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents("Step 2"))
	// Step 3 did not get to start in the first instance and ran in the second.
	require.Equal(t, `
{[1 5 SimpleTest Step 3][(*Target)(nil) TestStepRunningEvent]}
{[1 5 SimpleTest Step 3][(*Target)(nil) TestStepFinishedEvent]}
`, getStepEvents("Step 3"))
	// T1 failed entirely within the first run.
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestFailedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TargetErr &"{\"Error\":\"target failed\"}"]}
`, getTargetEvents("T1"))
	// T2 and T3 ran in both.
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TargetIn]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TestStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TestFinishedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} TargetOut]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TargetIn]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TestStartedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TestFinishedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TargetOut]}
{[1 5 SimpleTest Step 3][Target{ID: "T2"} TargetIn]}
{[1 5 SimpleTest Step 3][Target{ID: "T2"} TestStartedEvent]}
{[1 5 SimpleTest Step 3][Target{ID: "T2"} TestFinishedEvent]}
{[1 5 SimpleTest Step 3][Target{ID: "T2"} TargetOut]}
`, getTargetEvents("T2"))
}
