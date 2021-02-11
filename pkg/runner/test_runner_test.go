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
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/pluginregistry"
	"github.com/facebookincubator/contest/pkg/statectx"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/plugins/storage/memory"
	"github.com/facebookincubator/contest/plugins/teststeps/example"
	"github.com/facebookincubator/contest/tests/common"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/badtargets"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/channels"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/hanging"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/noreturn"
	"github.com/facebookincubator/contest/tests/plugins/teststeps/panicstep"
)

const (
	testName          = "SimpleTest"
	stepInjectTimeout = 3 * time.Second
	shutdownTimeout   = 3 * time.Second
)

var (
	evs            storage.ResettableStorage
	pluginRegistry *pluginregistry.PluginRegistry
)

func TestMain(m *testing.M) {
	flag.Parse()
	logging.Debug()
	if ms, err := memory.New(); err == nil {
		evs = ms
		if err := storage.SetStorage(ms); err != nil {
			panic(err.Error())
		}
	} else {
		panic(fmt.Sprintf("could not initialize in-memory storage layer: %v", err))
	}
	pluginRegistry = pluginregistry.NewPluginRegistry()
	for _, e := range []struct {
		name    string
		factory test.TestStepFactory
		events  []event.Name
	}{
		{badtargets.Name, badtargets.New, badtargets.Events},
		{channels.Name, channels.New, channels.Events},
		{example.Name, example.New, example.Events},
		{hanging.Name, hanging.New, hanging.Events},
		{noreturn.Name, noreturn.New, noreturn.Events},
		{panicstep.Name, panicstep.New, panicstep.Events},
	} {
		if err := pluginRegistry.RegisterTestStep(e.name, e.factory, e.events); err != nil {
			panic(fmt.Sprintf("could not register TestStep: %v", err))
		}
	}
	flag.Parse()
	common.LeakCheckingTestMain(m,
		// We expect these to leak.
		"github.com/facebookincubator/contest/tests/plugins/teststeps/hanging.(*hanging).Run",
		"github.com/facebookincubator/contest/tests/plugins/teststeps/noreturn.(*noreturnStep).Run",
	)
}

func newTestRunner() TestRunner {
	return NewTestRunnerWithTimeouts(stepInjectTimeout, shutdownTimeout)
}

func eventToStringNoTime(ev testevent.Event) string {
	// Omit the timestamp to make output stable.
	return fmt.Sprintf("{%s%s}", ev.Header, ev.Data)
}

func resetEventStorage() {
	if err := evs.Reset(); err != nil {
		panic(err.Error())
	}
}

func tgt(id string) *target.Target {
	return &target.Target{ID: id}
}

func getEvents(targetID, stepLabel *string) string {
	q, _ := testevent.BuildQuery(testevent.QueryTestName(testName))
	results, _ := evs.GetTestEvents(q)
	var resultsForTarget []string
	for _, r := range results {
		if targetID != nil {
			if r.Data.Target == nil {
				continue
			}
			if *targetID != "" && r.Data.Target.ID != *targetID {
				continue
			}
		}
		if stepLabel != nil {
			if *stepLabel != "" && r.Header.TestStepLabel != *stepLabel {
				continue
			}
			if targetID == nil && r.Data.Target != nil {
				continue
			}
		}
		resultsForTarget = append(resultsForTarget, eventToStringNoTime(r))
	}
	return "\n" + strings.Join(resultsForTarget, "\n") + "\n"
}

func getStepEvents(stepLabel string) string {
	return getEvents(nil, &stepLabel)
}

func getTargetEvents(targetID string) string {
	return getEvents(&targetID, nil)
}

func newStep(label, name string, params *test.TestStepParameters) test.TestStepBundle {
	td := test.TestStepDescriptor{
		Name:  name,
		Label: label,
	}
	if params != nil {
		td.Parameters = *params
	}
	sb, err := pluginRegistry.NewTestStepBundle(td, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create test step bundle: %v", err))
	}
	return *sb
}

func newExampleStep(label string, failPct int, failTargets string, delayTargets string) test.TestStepBundle {
	return newStep(label, example.Name, &test.TestStepParameters{
		example.FailPctParam:      []test.Param{*test.NewParam(fmt.Sprintf("%d", failPct))},
		example.FailTargetsParam:  []test.Param{*test.NewParam(failTargets)},
		example.DelayTargetsParam: []test.Param{*test.NewParam(delayTargets)},
	})
}

type runRes struct {
	res []byte
	err error
}

func runWithTimeout(t *testing.T, tr TestRunner, ctx statectx.Context, resumeState []byte, runID types.RunID, timeout time.Duration, targets []*target.Target, bundles []test.TestStepBundle) ([]byte, error) {
	newCtx, _, cancel := statectx.WithParent(ctx)
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
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newExampleStep("Step 1", 0, "", ""),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepFinishedEvent]}
`, getStepEvents(""))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleFinishedEvent]}
`, getTargetEvents("T1"))
}

// Simple case: one target, one step, failure.
func Test1Step1Fail(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newExampleStep("Step 1", 100, "", ""),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepFinishedEvent]}
`, getStepEvents("Step 1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleFailedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestError &"\"target failed\""]}
`, getTargetEvents("T1"))
}

// One step pipeline with two targets - one fails, one succeeds.
func Test1Step1Success1Fail(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1"), tgt("T2")},
		[]test.TestStepBundle{
			newExampleStep("Step 1", 0, "T1", "T2=100"),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepFinishedEvent]}
`, getStepEvents(""))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleFailedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestError &"\"target failed\""]}
`, getTargetEvents("T1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T2"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} ExampleFinishedEvent]}
`, getTargetEvents("T2"))
}

// Three-step pipeline, two targets: T1 fails at step 1, T2 fails at step 2,
// step 3 is not reached and not even run.
func Test3StepsNotReachedStepNotRun(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1"), tgt("T2")},
		[]test.TestStepBundle{
			newExampleStep("Step 1", 0, "T1", ""),
			newExampleStep("Step 2", 0, "T2", ""),
			newExampleStep("Step 3", 0, "", ""),
		},
	)
	require.NoError(t, err)
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepFinishedEvent]}
`, getStepEvents("Step 1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 2][(*Target)(nil) ExampleStepRunningEvent]}
{[1 1 SimpleTest Step 2][(*Target)(nil) ExampleStepFinishedEvent]}
`, getStepEvents("Step 2"))
	require.Equal(t, "\n\n", getStepEvents("Step 3"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleFailedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestError &"\"target failed\""]}
`, getTargetEvents("T1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T2"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} ExampleFinishedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} ExampleFailedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} TestError &"\"target failed\""]}
`, getTargetEvents("T2"))
}

// A misbehaving step that fails to shut down properly after processing targets
// and does not return.
func TestNoReturnStepWithCorrectTargetForwarding(t *testing.T) {
	resetEventStorage()
	tr := NewTestRunnerWithTimeouts(100*time.Millisecond, 200*time.Millisecond)
	ctx, _, cancel := statectx.New()
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

// A misbehaving step that does not process any targets.
func TestNoReturnStepWithoutTargetForwarding(t *testing.T) {
	resetEventStorage()
	tr := NewTestRunnerWithTimeouts(100*time.Millisecond, 200*time.Millisecond)
	ctx, _, cancel := statectx.New()
	defer cancel()
	_, err := runWithTimeout(t, tr, ctx, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newStep("Step 1", hanging.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestTargetInjectionTimedOut{}, err)
}

// A misbehaving step that panics.
func TestStepPanics(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newStep("Step 1", panicstep.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepPaniced{}, err)
	require.Equal(t, "\n\n", getTargetEvents("T1"))
}

// A misbehaving step that closes its output channel.
func TestStepClosesChannels(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newStep("Step 1", channels.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepClosedChannels{}, err)
	require.Equal(t, "\n\n", getTargetEvents("T1"))
}

// A misbehaving step that yields a result for a target that does not exist.
func TestStepYieldsResultForNonexistentTarget(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1")},
		[]test.TestStepBundle{
			newStep("Step 1", badtargets.Name, nil),
		},
	)
	require.Error(t, err)
}

// A misbehaving step that yields a result for a target that does not exist.
func TestStepYieldsDuplicateResult(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("TGood"), tgt("TDup")},
		[]test.TestStepBundle{
			// TGood makes it past here unscathed and gets delayed in Step 2,
			// TDup also emerges fine at first but is then returned again, and that's bad.
			newStep("Step 1", badtargets.Name, nil),
			newExampleStep("Step 2", 0, "", "TGood=100"),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepReturnedDuplicateResult{}, err)
}

// A misbehaving step that loses targets.
func TestStepLosesTargets(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("TGood"), tgt("TDrop")},
		[]test.TestStepBundle{
			newStep("Step 1", badtargets.Name, nil),
		},
	)
	require.Error(t, err)
	require.IsType(t, &cerrors.ErrTestStepLostTargets{}, err)
}

// A misbehaving step that yields a result for a target that does exist
// but is not currently waiting for it.
func TestStepYieldsResultForUnexpectedTarget(t *testing.T) {
	resetEventStorage()
	tr := newTestRunner()
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		[]*target.Target{tgt("T1"), tgt("T1XXX")},
		[]test.TestStepBundle{
			// T1XXX fails here.
			newExampleStep("Step 1", 0, "T1XXX", ""),
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
	_, err := runWithTimeout(t, tr, nil, nil, 1, 2*time.Second,
		targets,
		[]test.TestStepBundle{
			newExampleStep("Step 1", 0, "", "*=10"),  // All targets pass the first step, with a slight delay
			newExampleStep("Step 2", 25, "", ""),     // 25% don't make it past the second step
			newExampleStep("Step 3", 25, "", "*=10"), // Another 25% fail at the third step
		},
	)
	require.NoError(t, err)
	// Every target mush have started and finished the first step.
	numFinished := 0
	for _, tgt := range targets {
		s1n := "Step 1"
		require.Equal(t, fmt.Sprintf(`
{[1 1 SimpleTest Step 1][Target{ID: "%s"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "%s"} ExampleFinishedEvent]}
`, tgt.ID, tgt.ID),
			getEvents(&tgt.ID, &s1n))
		s3n := "Step 3"
		if strings.Contains(getEvents(&tgt.ID, &s3n), "ExampleFinishedEvent") {
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
	log := logging.GetLogger("TestPauseResumeSimple")
	var err error
	var resumeState []byte
	targets := []*target.Target{tgt("T1"), tgt("T2"), tgt("T3")}
	steps := []test.TestStepBundle{
		newExampleStep("Step 1", 0, "T1", ""),
		// T2 and T3 will be paused here, the step will be given time to finish.
		newExampleStep("Step 2", 0, "", "T2=200,T3=200"),
		newExampleStep("Step 3", 0, "", ""),
	}
	{
		tr1 := newTestRunner()
		ctx1, pause, cancel := statectx.New()
		defer cancel()
		go func() {
			time.Sleep(100 * time.Millisecond)
			log.Infof("TestPauseResumeNaive: pausing")
			pause()
		}()
		resumeState, err = runWithTimeout(t, tr1, ctx1, nil, 1, 2*time.Second, targets, steps)
		require.Error(t, err)
		require.IsType(t, statectx.ErrPaused, err)
		require.NotNil(t, resumeState)
	}
	log.Debugf("Resume state: %s", string(resumeState))
	// Make sure that resume state is validated.
	{
		tr := newTestRunner()
		ctx, _, cancel := statectx.New()
		defer cancel()
		resumeState2, err := runWithTimeout(
			t, tr, ctx, []byte("FOO"), 2, 2*time.Second, targets, steps)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid resume state")
		require.Nil(t, resumeState2)
	}
	{
		tr := newTestRunner()
		ctx, _, cancel := statectx.New()
		defer cancel()
		resumeState2 := strings.Replace(string(resumeState), `"version"`, `"Xversion"`, 1)
		_, err := runWithTimeout(
			t, tr, ctx, []byte(resumeState2), 3, 2*time.Second, targets, steps)
		require.Error(t, err)
		require.Contains(t, err.Error(), "incompatible resume state")
	}
	{
		tr := newTestRunner()
		ctx, _, cancel := statectx.New()
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
		ctx2, _, cancel := statectx.New()
		defer cancel()
		_, err := runWithTimeout(t, tr2, ctx2, resumeState, 5, 2*time.Second,
			// Pass exactly the same targets and pipeline to resume properly.
			// Don't use the same pointers ot make sure there is no reliance on that.
			[]*target.Target{tgt("T1"), tgt("T2"), tgt("T3")},
			[]test.TestStepBundle{
				newExampleStep("Step 1", 0, "T1", ""),
				newExampleStep("Step 2", 0, "", "T2=200,T3=200"),
				newExampleStep("Step 3", 0, "", ""),
			},
		)
		require.NoError(t, err)
	}
	// Verify step events.
	// Steps 1 and 2 are executed entirely within the first runner instance
	// and never started in the second.
	require.Equal(t, `
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepRunningEvent]}
{[1 1 SimpleTest Step 1][(*Target)(nil) ExampleStepFinishedEvent]}
`, getStepEvents("Step 1"))
	require.Equal(t, `
{[1 1 SimpleTest Step 2][(*Target)(nil) ExampleStepRunningEvent]}
{[1 1 SimpleTest Step 2][(*Target)(nil) ExampleStepFinishedEvent]}
`, getStepEvents("Step 2"))
	// Step 3 did not get to start in the first instance and ran in the second.
	require.Equal(t, `
{[1 5 SimpleTest Step 3][(*Target)(nil) ExampleStepRunningEvent]}
{[1 5 SimpleTest Step 3][(*Target)(nil) ExampleStepFinishedEvent]}
`, getStepEvents("Step 3"))
	// T1 failed entirely within the first run.
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} ExampleFailedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T1"} TestError &"\"target failed\""]}
`, getTargetEvents("T1"))
	// T2 and T3 ran in both.
	require.Equal(t, `
{[1 1 SimpleTest Step 1][Target{ID: "T2"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 1][Target{ID: "T2"} ExampleFinishedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} ExampleStartedEvent]}
{[1 1 SimpleTest Step 2][Target{ID: "T2"} ExampleFinishedEvent]}
{[1 5 SimpleTest Step 3][Target{ID: "T2"} ExampleStartedEvent]}
{[1 5 SimpleTest Step 3][Target{ID: "T2"} ExampleFinishedEvent]}
`, getTargetEvents("T2"))
}
