// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package common

import (
	"fmt"
	"strings"

	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/storage"
	"github.com/facebookincubator/contest/pkg/xcontext"
)

func eventToStringNoTime(ev testevent.Event) string {
	// Omit the timestamp to make output stable.
	return fmt.Sprintf("{%s%s}", ev.Header, ev.Data)
}

// GetTestEventsAsString queries storage for particular test's events,
// further filtering by target ID and/or step label.
func GetTestEventsAsString(ctx xcontext.Context, st storage.EventStorage, testName string, targetID, stepLabel *string) string {
	q, _ := testevent.BuildQuery(testevent.QueryTestName(testName))
	results, _ := st.GetTestEvents(ctx, q)
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
