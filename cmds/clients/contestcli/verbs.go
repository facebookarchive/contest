// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/transport"
	"github.com/facebookincubator/contest/pkg/types"

	flag "github.com/spf13/pflag"
)

func run(requestor string, transport transport.Transport) error {
	verb := strings.ToLower(flag.Arg(0))
	if verb == "" {
		fmt.Fprintf(flag.CommandLine.Output(), "Missing verb, see --help\n")
		os.Exit(1)
	}
	var resp interface{}
	var err error
	switch verb {
	case "start":
		fmt.Fprintf(os.Stderr, "Reading from stdin...\n")
		jobDesc, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read job descriptor: %v", err)
		}

		jobDescFormat := config.JobDescFormatJSON
		if *flagYAML {
			jobDescFormat = config.JobDescFormatYAML
		}
		jobDescJSON, err := config.ParseJobDescriptor(jobDesc, jobDescFormat)
		if err != nil {
			return fmt.Errorf("failed to parse job descriptor: %w", err)
		}

		startResp, err := transport.Start(context.Background(), requestor, string(jobDescJSON))
		if err != nil {
			return err
		}
		resp = startResp

		// handle wait
		if *flagWait && startResp.Data.JobID != 0 {
			// print immediately if wait is used
			buffer := &bytes.Buffer{}
			encoder := json.NewEncoder(buffer)
			encoder.SetEscapeHTML(false)
			encoder.SetIndent("", " ")
			err = encoder.Encode(startResp)
			if err != nil {
				return fmt.Errorf("cannot re-encode api.Respose object: %v", err)
			}
			indentedJSON := buffer.String()
			fmt.Println(indentedJSON)

			fmt.Fprintf(os.Stderr, "\nWaiting for job to complete...\n")
			resp, err = wait(context.Background(), startResp.Data.JobID, jobWaitPoll, requestor, transport)
			if err != nil {
				return err
			}
		}
	case "stop":
		jobID, err := parseJob(flag.Arg(1))
		if err != nil {
			return err
		}
		resp, err = transport.Stop(context.Background(), requestor, types.JobID(jobID))
		if err != nil {
			return err
		}
	case "status":
		jobID, err := parseJob(flag.Arg(1))
		if err != nil {
			return err
		}
		resp, err = transport.Status(context.Background(), requestor, jobID)
		if err != nil {
			return err
		}
	case "retry":
		jobID, err := parseJob(flag.Arg(1))
		if err != nil {
			return err
		}
		resp, err = transport.Retry(context.Background(), requestor, jobID)
		if err != nil {
			return err
		}
	case "list":
		var states []job.State
		for _, sts := range *flagStates {
			st, err := job.EventNameToJobState(event.Name(sts))
			if err != nil {
				return err
			}
			states = append(states, st)
		}
		resp, err = transport.List(context.Background(), requestor, states, *flagTags)
		if err != nil {
			return err
		}
	case "version":
		resp, err = transport.Version(context.Background(), requestor)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid verb: '%s'", verb)
	}
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", " ")
	err = encoder.Encode(resp)
	if err != nil {
		return fmt.Errorf("cannot re-encode api.Respose object: %v", err)
	}
	fmt.Println(buffer.String())
	return nil
}

func wait(ctx context.Context, jobID types.JobID, jobWaitPoll time.Duration, requestor string, transport transport.Transport) (*api.StatusResponse, error) {
	// keep polling for status till job is completed, used when -wait is set
	for {
		resp, err := transport.Status(context.Background(), requestor, jobID)
		if err != nil {
			return nil, err
		}
		if resp.Err != nil {
			return nil, fmt.Errorf("server responded with an error: %s", resp.Err)
		}

		jobState := resp.Data.Status.State

		for _, eventName := range job.JobCompletionEvents {
			if string(jobState) == string(eventName) {
				return resp, nil
			}
		}
		// TODO use  time.Ticker instead of time.Sleep
		time.Sleep(jobWaitPoll)
	}
}

func parseJob(jobIDStr string) (types.JobID, error) {
	if jobIDStr == "" {
		return 0, errors.New("missing job ID")
	}
	var jobID types.JobID
	jobIDint, err := strconv.Atoi(jobIDStr)
	if err != nil {
		return 0, fmt.Errorf("Invalid job ID: %s: %v", jobIDStr, err)
	}
	jobID = types.JobID(jobIDint)
	if jobID <= 0 {
		return 0, fmt.Errorf("Invalid job ID: %s: it must be positive", jobIDStr)
	}
	return jobID, nil
}
