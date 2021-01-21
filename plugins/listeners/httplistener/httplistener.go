// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package httplistener

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/api"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/job"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/types"
)

var log = logging.GetLogger("listeners/httplistener")

// HTTPListener implements the api.Listener interface.
type HTTPListener struct {
}

// HTTPAPIResponse is returned when an API method succeeds. It wraps the content
// of an api.Response and reworks some of its fields
type HTTPAPIResponse struct {
	ServerID string
	// the original type is ResponseType. Here we want the mnemonic string to
	// return in the HTTP API response.
	Type  string
	Data  interface{}
	Error *string
}

// NewHTTPAPIResponse returns an HTTPAPIResponse from an api.Response object. In
// case of errors, some fields are set accordingly.
func NewHTTPAPIResponse(r *api.Response) *HTTPAPIResponse {
	rtype, ok := api.ResponseTypeToName[r.Type]
	if !ok {
		rtype = fmt.Sprintf("unknown (%d)", r.Type)
	}
	var errStr *string
	if r.Err != nil {
		e := r.Err.Error()
		errStr = &e
	}
	return &HTTPAPIResponse{
		ServerID: r.ServerID,
		Type:     rtype,
		Data:     r.Data,
		Error:    errStr,
	}
}

// HTTPAPIError is returned when an API method fails. It wraps the error
// message.
type HTTPAPIError struct {
	Msg string
}

func strToJobID(s string) (types.JobID, error) {
	if strings.TrimSpace(s) == "" {
		return 0, errors.New("job ID cannot be empty")
	}
	jobIDInt, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return types.JobID(jobIDInt), nil
}

type apiHandler struct {
	api *api.API
}

func reply(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	if _, err := fmt.Fprint(w, msg); err != nil {
		log.Printf("Cannot write to client socket: %v", err)
	}
}

func (h *apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	verb := strings.TrimLeft(r.URL.Path, "/")
	var (
		httpStatus = http.StatusOK
		resp       api.Response
		errMsg     string
		err        error
	)
	// This is only used by status, stop, and reply. Ignored for other
	// methods. If not set by the client, this is an empty string.
	if r.Method != "POST" {
		reply(w, http.StatusBadRequest, "Only POST requests are supported")
		return
	}
	jobIDStr := r.PostFormValue("jobID")
	jobDesc := r.PostFormValue("jobDesc")
	requestor := api.EventRequestor(r.PostFormValue("requestor"))

	switch verb {
	case "start":
		if jobDesc == "" {
			httpStatus = http.StatusBadRequest
			errMsg = "Missing job description"
			break
		}
		if resp, err = h.api.Start(requestor, jobDesc); err != nil {
			httpStatus = http.StatusBadRequest
			errMsg = fmt.Sprintf("Start failed: %v", err)
		}
	case "status":
		jobID, err := strToJobID(jobIDStr)
		if err != nil {
			httpStatus = http.StatusBadRequest
			errMsg = fmt.Sprintf("Status failed: %v", err)
			break
		}
		if resp, err = h.api.Status(requestor, jobID); err != nil {
			httpStatus = http.StatusBadRequest
			errMsg = fmt.Sprintf("Status failed: %v", err)
		}
	case "stop":
		jobID, err := strToJobID(jobIDStr)
		if err != nil {
			httpStatus = http.StatusBadRequest
			errMsg = fmt.Sprintf("Stop failed: %v", err)
			break
		}
		if resp, err = h.api.Stop(requestor, jobID); err != nil {
			httpStatus = http.StatusBadRequest
			errMsg = fmt.Sprintf("Stop failed: %v", err)
		}
	case "retry":
		jobID, err := strToJobID(jobIDStr)
		if err != nil {
			httpStatus = http.StatusBadRequest
			errMsg = fmt.Sprintf("Retry failed: %v", err)
			break
		}
		if resp, err = h.api.Retry(requestor, jobID); err != nil {
			httpStatus = http.StatusBadRequest
			errMsg = fmt.Sprintf("Retry failed: %v", err)
		}
	case "list":
		var states []job.State
		var tags []string
		if statesStr := r.PostFormValue("states"); len(statesStr) > 0 {
			for _, sts := range strings.Split(statesStr, ",") {
				st, err := job.EventNameToJobState(event.Name(sts))
				if err != nil {
					httpStatus = http.StatusBadRequest
					errMsg = fmt.Sprintf("List failed: %v", err)
					break
				}
				states = append(states, st)
			}
		}
		if tagsStr := r.PostFormValue("tags"); len(tagsStr) > 0 {
			tags = strings.Split(tagsStr, ",")
		}
		if resp, err = h.api.List(requestor, states, tags); err != nil {
			httpStatus = http.StatusBadRequest
			errMsg = fmt.Sprintf("List failed: %v", err)
		}
	case "version":
		resp = h.api.Version()
	default:
		errMsg = fmt.Sprintf("unknown verb: %s", verb)
		httpStatus = http.StatusBadRequest
	}
	if httpStatus != http.StatusOK {
		errResp := HTTPAPIError{
			Msg: errMsg,
		}
		msg, err := json.Marshal(errResp)
		if err != nil {
			panic(fmt.Sprintf("cannot marshal HTTPAPIError: %v", err))
		}
		reply(w, httpStatus, string(msg))
		return
	}
	apiResp := NewHTTPAPIResponse(&resp)

	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	err = encoder.Encode(apiResp)
	if err != nil {
		panic(fmt.Sprintf("cannot marshal HTTPAPIResponse: %v", err))
	}
	msg := buffer.Bytes()
	reply(w, httpStatus, string(msg))
}

func listenWithCancellation(cancel <-chan struct{}, s *http.Server) error {
	var (
		errCh = make(chan error, 1)
	)
	// start the listener asynchronously, and report errors and completion via
	// channels.
	go func() {
		errCh <- s.ListenAndServe()
	}()
	log.Infof("Started HTTP API listener on %s", s.Addr)
	// wait for cancellation or for completion
	select {
	case err := <-errCh:
		return err
	case <-cancel:
		log.Printf("Received server shut down request")
		return s.Close()
	}
}

// Serve implements the api.Listener.Serve interface method. It starts an HTTP
// API listener and returns an api.Event channel that the caller can iterate on.
func (h *HTTPListener) Serve(cancel <-chan struct{}, a *api.API) error {
	if a == nil {
		return errors.New("API object is nil")
	}
	s := http.Server{
		Addr:         ":8080",
		Handler:      &apiHandler{api: a},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	if err := listenWithCancellation(cancel, &s); err != nil {
		return fmt.Errorf("HTTP listener failed: %v", err)
	}
	log.Printf("Server shut down successfully.")
	return nil
}
