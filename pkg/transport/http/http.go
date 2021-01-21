// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/types"
	"github.com/facebookincubator/contest/plugins/listeners/httplistener"

	"github.com/insomniacslk/xjson"
)

// HttpPartiallyDecodedResponse is a httplistener.HTTPAPIResponse, but with the Data not fully decoded yet
type HTTPPartiallyDecodedResponse struct {
	ServerID string
	Type     string
	Data     json.RawMessage
	Error    *xjson.Error
}

// HTTP communicates with ConTest Server via http(s)/json transport
// HTTP implements the Transport interface
type HTTP struct {
	Addr string
}

func (h *HTTP) Version(ctx context.Context, requestor string) (*api.VersionResponse, error) {
	resp, err := h.request(requestor, "version", url.Values{})
	if err != nil {
		return nil, err
	}
	data := api.ResponseDataVersion{}
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		return nil, fmt.Errorf("cannot decode json response: %v", err)
	}
	return &api.VersionResponse{ServerID: resp.ServerID, Data: data, Err: resp.Error}, nil
}

func (h *HTTP) Start(ctx context.Context, requestor string, jobDescriptor string) (*api.StartResponse, error) {
	params := url.Values{}
	params.Add("jobDesc", jobDescriptor)
	resp, err := h.request(requestor, "start", params)
	if err != nil {
		return nil, err
	}
	data := api.ResponseDataStart{}
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		return nil, fmt.Errorf("cannot decode json response: %v", err)
	}
	return &api.StartResponse{ServerID: resp.ServerID, Data: data, Err: resp.Error}, nil
}

func (h *HTTP) Stop(ctx context.Context, requestor string, jobID types.JobID) (*api.StopResponse, error) {
	params := url.Values{}
	params.Add("jobID", strconv.Itoa(int(jobID)))
	resp, err := h.request(requestor, "stop", params)
	if err != nil {
		return nil, err
	}
	data := api.ResponseDataStop{}
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		return nil, fmt.Errorf("cannot decode json response: %v", err)
	}
	return &api.StopResponse{ServerID: resp.ServerID, Data: data, Err: resp.Error}, nil
}

func (h *HTTP) Status(ctx context.Context, requestor string, jobID types.JobID) (*api.StatusResponse, error) {
	params := url.Values{}
	params.Add("jobID", strconv.Itoa(int(jobID)))
	resp, err := h.request(requestor, "status", params)
	if err != nil {
		return nil, err
	}
	data := api.ResponseDataStatus{}
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		return nil, fmt.Errorf("cannot decode json response: %v", err)
	}
	return &api.StatusResponse{ServerID: resp.ServerID, Data: data, Err: resp.Error}, nil
}

func (h *HTTP) Retry(ctx context.Context, requestor string, jobID types.JobID) (*api.RetryResponse, error) {
	params := url.Values{}
	params.Add("jobID", strconv.Itoa(int(jobID)))
	resp, err := h.request(requestor, "retry", params)
	if err != nil {
		return nil, err
	}
	data := api.ResponseDataRetry{}
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		return nil, fmt.Errorf("cannot decode json response: %v", err)
	}
	return &api.RetryResponse{ServerID: resp.ServerID, Data: data, Err: resp.Error}, nil
}

func (h *HTTP) List(ctx context.Context, requestor string, states []job.State, tags []string) (*api.ListResponse, error) {
	params := url.Values{}
	if len(states) > 0 {
		sts := make([]string, len(states))
		for i, st := range states {
			sts[i] = st.String()
		}
		params.Set("states", strings.Join(sts, ","))
	}
	if len(tags) > 0 {
		params.Set("tags", strings.Join(tags, ","))
	}
	resp, err := h.request(requestor, "list", params)
	if err != nil {
		return nil, err
	}
	var data api.ResponseDataList
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		return nil, fmt.Errorf("cannot decode json response: %v", err)
	}
	return &api.ListResponse{ServerID: resp.ServerID, Data: data, Err: resp.Error}, nil
}

func (h *HTTP) request(requestor string, verb string, params url.Values) (*HTTPPartiallyDecodedResponse, error) {
	params.Set("requestor", requestor)
	u, err := url.Parse(h.Addr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server address '%s': %v", h.Addr, err)
	}
	if u.Scheme == "" {
		return nil, errors.New("server URL scheme not specified")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme '%s', please specify either http or https", u.Scheme)
	}
	u.Path += "/" + verb
	fmt.Fprintf(os.Stderr, "Requesting URL %s with requestor ID '%s'\n", u.String(), requestor)
	fmt.Fprintf(os.Stderr, "  with params:\n")
	for k, v := range params {
		fmt.Fprintf(os.Stderr, "    %s: %s\n", k, v)
	}
	fmt.Fprintf(os.Stderr, "\n")
	resp, err := http.PostForm(u.String(), params)
	if err != nil {
		return nil, fmt.Errorf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read HTTP response: %v", err)
	}
	fmt.Fprintf(os.Stderr, "The server responded with status %s\n", resp.Status)

	var apiResp HTTPPartiallyDecodedResponse
	if resp.StatusCode == http.StatusOK {
		// the Data field of apiResp will result in a map[string]interface{}
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return nil, fmt.Errorf("response is not a valid HTTP API response object: '%s': %v", body, err)
		}
		if err != nil {
			return nil, fmt.Errorf("cannot marshal HTTPAPIResponse: %v", err)
		}
	} else {
		var apiErr httplistener.HTTPAPIError
		if err := json.Unmarshal(body, &apiErr); err != nil {
			return nil, fmt.Errorf("response is not a valid HTTP API Error object: '%s': %v", body, err)
		}
		apiResp.Error = xjson.NewError(errors.New(apiErr.Msg))
	}

	return &apiResp, nil
}
