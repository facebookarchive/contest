// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/config"
	"github.com/facebookincubator/contest/pkg/jobmanager"
	"github.com/facebookincubator/contest/plugins/listeners/httplistener"

	flag "github.com/spf13/pflag"
)

// Unauthenticated, unencrypted sample HTTP client for ConTest.
// Requires the `httplistener` plugin for the API listener.
//
// Usage examples:
// Start a job with the provided job description from a JSON file
//   ./contestcli-http start < start.json
//
// Get the status of a job whose ID is 10
//   ./contestcli-http status 10

const (
	defaultRequestor = "contestcli-http"
	jobWaitPoll      = 10 * time.Second
)

var (
	flagAddr      = flag.StringP("addr", "a", "http://localhost:8080", "ConTest server [scheme://]host:port[/basepath] to connect to")
	flagRequestor = flag.StringP("requestor", "r", defaultRequestor, "Identifier of the requestor of the API call")
	flagWait      = flag.BoolP("wait", "w", false, "After starting a job, wait for it to finish, and exit 0 only if it is successful")
	flagYAML      = flag.BoolP("yaml", "Y", false, "Parse job descriptor as YAML instead of JSON")
	flagS3        = flag.BoolP("s3", "s", false, "Upload Job Result to S3 Bucket")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of contestcli-http:\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  contestcli-http [args] command\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "command: start, stop, status, retry, version\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  start\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        start a new job using the job description passed via stdin\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        when used with -wait flag, stdout will have two JSON outputs\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        for job start and completion status separated with newline\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  stop int\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        stop a job by job ID\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  status int\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        get the status of a job by job ID\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  retry int\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        retry a job by job ID\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  version\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        request the API version to the server\n")
		fmt.Fprintf(flag.CommandLine.Output(), "\nargs:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	verb := strings.TrimSpace(flag.Arg(0))
	if verb == "" {
		fmt.Fprintf(flag.CommandLine.Output(), "Missing verb, see --help\n")
		os.Exit(1)
	}
	if err := run(verb); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(verb string) error {
	var (
		params = url.Values{}
	)
	params.Set("requestor", *flagRequestor)
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
		params.Add("jobDesc", string(jobDescJSON))
		resp, err := request(verb, params)
		if err != nil {
			return err
		}
		fmt.Println(resp)

		if *flagWait {
			fmt.Fprintf(os.Stderr, "\nWaiting for job to complete...\n")
			parsedData := &api.ResponseDataStart{}
			parsedResp := &httplistener.HTTPAPIResponse{Data: parsedData}
			if err := json.Unmarshal([]byte(resp), parsedResp); err != nil {
				return fmt.Errorf("cannot decode json response: %v", err)
			}
			jobID := parsedData.JobID
			params.Set("jobID", strconv.Itoa(int(jobID)))

			resp, err = wait(params, jobWaitPoll)
			if err != nil {
				return err
			}

			if *flagS3 {
				err = pushResultsToS3(jobID, resp)
				if err != nil {
					return err
				}
			}

			fmt.Println(resp)
		}
	case "stop", "status", "retry":
		jobID := flag.Arg(1)
		if jobID == "" {
			return errors.New("missing job ID")
		}
		params.Set("jobID", jobID)
		resp, err := request(verb, params)
		if err != nil {
			return err
		}
		fmt.Println(resp)
	case "version":
		// no params for protocol version
	default:
		return fmt.Errorf("invalid verb: '%s'", verb)
	}
	return nil
}

func request(verb string, params url.Values) (string, error) {
	u, err := url.Parse(*flagAddr)
	if err != nil {
		return "", fmt.Errorf("failed to parse server address '%s': %v", *flagAddr, err)
	}
	if u.Scheme == "" {
		return "", errors.New("server URL scheme not specified")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme '%s', please specify either http or https", u.Scheme)
	}
	u.Path += "/" + verb
	fmt.Fprintf(os.Stderr, "Requesting URL %s with requestor ID '%s'\n", u.String(), *flagRequestor)
	fmt.Fprintf(os.Stderr, "  with params:\n")
	for k, v := range params {
		fmt.Fprintf(os.Stderr, "    %s: %s\n", k, v)
	}
	fmt.Fprintf(os.Stderr, "\n")
	resp, err := http.PostForm(u.String(), params)
	if err != nil {
		return "", fmt.Errorf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Cannot read HTTP response: %v", err)
	}
	fmt.Fprintf(os.Stderr, "The server responded with status %s", resp.Status)
	var indentedJSON []byte
	if resp.StatusCode == http.StatusOK {
		// the Data field of apiResp will result in a map[string]interface{}
		var apiResp httplistener.HTTPAPIResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return "", fmt.Errorf("response is not a valid HTTP API response object: '%s': %v", body, err)
		}
		// re-encode and indent, for pretty-printing

		if err != nil {
			return "", fmt.Errorf("cannot marshal HTTPAPIResponse: %v", err)
		}

		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", " ")
		err := encoder.Encode(apiResp)
		if err != nil {
			return "", fmt.Errorf("cannot re-encode httplistener.HTTPAPIResponse object: %v", err)
		}
		indentedJSON = buffer.Bytes()
	} else {
		var apiErr httplistener.HTTPAPIError
		if err := json.Unmarshal(body, &apiErr); err != nil {
			return "", fmt.Errorf("response is not a valid HTTP API Error object: '%s': %v", body, err)
		}
		// re-encode and indent, for pretty-printing
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", " ")
		err := encoder.Encode(apiErr)
		if err != nil {
			return "", fmt.Errorf("cannot re-encode httplistener.HTTPAPIResponse object: %v", err)
		}
		indentedJSON = buffer.Bytes()
	}
	// this is the only thing we want on stdout - the JSON-formatted response,
	// so it can be piped to other tools if desired.
	return string(indentedJSON), nil
}

func wait(params url.Values, jobWaitPoll time.Duration) (string, error) {
	// keep polling for status till job is completed, used when -wait is set
	for {
		resp, err := request("status", params)
		if err != nil {
			return "", err
		}

		parsedData := &api.ResponseDataStatus{}
		parsedResp := &httplistener.HTTPAPIResponse{Data: parsedData}
		if err := json.Unmarshal([]byte(resp), parsedResp); err != nil {
			return "", fmt.Errorf("cannot decode json response: %v", err)
		}
		if parsedResp.Error != nil {
			return "", fmt.Errorf("server responded with an error: %s", *parsedResp.Error)
		}
		jobState := parsedData.Status.State

		for _, eventName := range jobmanager.JobCompletionEvents {
			if string(jobState) == string(eventName) {
				return resp, nil
			}
		}
		// TODO use  time.Ticker instead of time.Sleep
		time.Sleep(jobWaitPoll)
	}
}
