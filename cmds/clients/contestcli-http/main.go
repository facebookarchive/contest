// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/facebookincubator/contest/plugins/listeners/httplistener"
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
)

var (
	flagAddr      = flag.String("addr", "http://localhost:8080", "ConTest server [scheme://]host:port[/basepath] to connect to")
	flagRequestor = flag.String("r", defaultRequestor, "Identifier of the requestor of the API call")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of contestcli-http:\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  contestcli-http [args] command\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "command: start, stop, status, retry, version\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  start\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        start a new job using the job description passed via stdin\n")
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
	verb := flag.Arg(0)
	if err := run(verb); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
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
			return fmt.Errorf("failed to parse job descriptor: %v", err)
		}
		params.Add("jobDesc", string(jobDesc))
	case "stop", "status", "retry":
		jobID := flag.Arg(1)
		if jobID == "" {
			return errors.New("missing job ID")
		}
		params.Set("jobID", jobID)
	case "version":
		// no params for protocol version
	default:
		return fmt.Errorf("invalid verb: '%s'", verb)
	}
	u, err := url.Parse(*flagAddr)
	if err != nil {
		return fmt.Errorf("failed to parse server address '%s': %v", *flagAddr, err)
	}
	if u.Scheme == "" {
		return errors.New("server URL scheme not specified")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme '%s', please specify either http or https", u.Scheme)
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
		return fmt.Errorf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Cannot read HTTP response: %v", err)
	}
	fmt.Fprintf(os.Stderr, "The server responded with status %s", resp.Status)
	var indentedJSON []byte
	if resp.StatusCode == http.StatusOK {
		// the Data field of apiResp will result in a map[string]interface{}
		var apiResp httplistener.HTTPAPIResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return fmt.Errorf("response is not a valid HTTP API response object: '%s': %v", body, err)
		}
		// re-encode and indent, for pretty-printing

		if err != nil {
			return fmt.Errorf("cannot marshal HTTPAPIResponse: %v", err)
		}

		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", " ")
		err := encoder.Encode(apiResp)
		if err != nil {
			return fmt.Errorf("cannot re-encode httplistener.HTTPAPIResponse object: %v", err)
		}
		indentedJSON = buffer.Bytes()
	} else {
		var apiErr httplistener.HTTPAPIError
		if err := json.Unmarshal(body, &apiErr); err != nil {
			return fmt.Errorf("response is not a valid HTTP API Error object: '%s': %v", body, err)
		}
		// re-encode and indent, for pretty-printing
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", " ")
		err := encoder.Encode(apiErr)
		if err != nil {
			return fmt.Errorf("cannot re-encode httplistener.HTTPAPIResponse object: %v", err)
		}
		indentedJSON = buffer.Bytes()
	}
	// this is the only thing we want on stdout - the JSON-formatted response,
	// so it can be piped to other tools if desired.
	fmt.Println(string(indentedJSON))
	return nil
}
