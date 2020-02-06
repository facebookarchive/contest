// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package uri

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/insomniacslk/xjson"
)

// Name defined the name of the plugin
var (
	Name = "URI"
	log  = logging.GetLogger("testfetchers/" + strings.ToLower(Name))
)

var supportedSchemes = []string{
	"file",
	"https",
	"http",
}

// FetchParameters contains the parameters necessary to fetch tests. This
// structure is populated from a JSON blob.
type FetchParameters struct {
	TestName string
	// URI is the string pointing to where the test definition is stored. At
	// the moment only file://, https:// and http:// are supported.
	URI *xjson.URL
}

// URI implements contest.TestFetcher interface, returning dummy test fetcher
type URI struct {
}

// ValidateFetchParameters performs sanity checks on the fields of the
// parameters that will be passed to Fetch.
func (tf URI) ValidateFetchParameters(params []byte) (interface{}, error) {
	var fp FetchParameters
	if err := json.Unmarshal(params, &fp); err != nil {
		return nil, err
	}
	if fp.TestName == "" {
		return nil, fmt.Errorf("test name cannot be empty for fetch parameters")
	}
	if fp.URI == nil {
		return nil, fmt.Errorf("file URI not specified in fetch parameters")
	}
	scheme := fp.URI.Scheme
	if scheme == "" {
		// if no scheme is specified, assume "file://"
		scheme = "file"
	}
	scheme = strings.ToLower(scheme)
	supported := false
	for _, s := range supportedSchemes {
		if s == scheme {
			supported = true
		}
	}
	if !supported {
		return nil, fmt.Errorf("unsupported scheme %s", scheme)
	}
	if scheme == "file" {
		if fp.URI.Host != "" && fp.URI.Host != "localhost" {
			return nil, fmt.Errorf("invalid host in URI: '%s'. Only 'localhost' or empty string are supported for scheme %s", fp.URI.Host, scheme)
		}
	}
	return fp, nil
}

// Fetch returns the information necessary to build a Test object. The returned
// values are:
// * Name of the test
// * list of step definitions
// * an error if any
func (tf *URI) Fetch(params interface{}) (string, []*test.TestStepDescriptor, error) {
	fetchParams, ok := params.(FetchParameters)
	if !ok {
		return "", nil, fmt.Errorf("Fetch expects uri.FetchParameters object")
	}
	log.Printf("Fetching tests with params %+v", fetchParams)
	scheme := strings.ToLower(strings.ToLower(fetchParams.URI.Scheme))
	var (
		buf []byte
		err error
	)
	switch scheme {
	case "", "file":
		// naively assume that it's OK to read the whole file in memory.
		buf, err = ioutil.ReadFile(fetchParams.URI.Path)
		if err != nil {
			return "", nil, err
		}
	case "http", "https":
		resp, err := http.Get(fetchParams.URI.String())
		if err != nil {
			return "", nil, err
		}
		buf, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", nil, err
		}
	default:
		return "", nil, fmt.Errorf("unsupported scheme '%s'", scheme)
	}
	type doc struct {
		Steps []*test.TestStepDescriptor
	}
	var d doc
	if err := json.Unmarshal(buf, &d); err != nil {
		return "", nil, fmt.Errorf("cannot decode JSON test description: %v", err)
	}
	// TODO do something with the Report object (or factor it out from the step
	//      definition)
	return fetchParams.TestName, d.Steps, nil
}

// New initializes the TestFetcher object
func New() test.TestFetcher {
	return &URI{}
}

// Load returns the name and factory which are needed to register the
// TestFetcher.
func Load() (string, test.TestFetcherFactory) {
	return Name, New
}
