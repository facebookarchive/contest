// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package awsfileupload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/pkg/xcontext"
	"github.com/facebookincubator/contest/plugins/teststeps"
)

// Name is the name used to look this plugin up.
var Name = "awsFileUpload"

// Event names for this plugin.
const (
	EventURLtoReport = event.Name("URLtoReport")
)

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{
	EventURLtoReport,
}

// FileUpload is used to retrieve all the parameter, the plugin needs.
type FileUpload struct {
	path            *test.Param // Path to file that shall be uploaded
	filename        *test.Param // Filename to file that shall be uploaded
	s3region        string      // AWS server region that shall be used
	s3bucket        string      // AWS Bucket name
	s3path          string      // Path where in the bucket to upload the file
	s3credfile      string      // Path to the AWS credential file to authenticate the session
	s3credprofile   string      // Profile of the AWS credential file that shall be used
	emitURLtoReport bool        // Bool that defines that URLtoReport Event shall be triggered
	compress        bool        // Bool that defines if the data should be compressed before uploading it
}

// Datatype for the emmiting the URLtoReport Event
type eventURLtoReportPayload struct {
	Msg string
}

// Name returns the plugin name.
func (ts FileUpload) Name() string {
	return Name
}

func emitEvent(ctx xcontext.Context, name event.Name, payload interface{}, tgt *target.Target, ev testevent.Emitter) error {
	payloadStr, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cannot encode payload for event '%s': %v", name, err)
	}
	rm := json.RawMessage(payloadStr)
	evData := testevent.Data{
		EventName: name,
		Target:    tgt,
		Payload:   &rm,
	}
	if err := ev.Emit(ctx, evData); err != nil {
		return fmt.Errorf("cannot emit event EventURLtoReport: %v", err)
	}
	return nil
}

// Run executes the awsFileUpload.
func (ts *FileUpload) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters,
	ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {
	// Validate the parameter
	if err := ts.validateAndPopulate(params); err != nil {
		return nil, err
	}
	f := func(ctx xcontext.Context, target *target.Target) error {
		// expand args
		path, err := ts.path.Expand(target)
		if err != nil {
			return fmt.Errorf("failed to expand argument '%s': %v", ts.path, err)
		}
		filename, err := ts.filename.Expand(target)
		if err != nil {
			return fmt.Errorf("failed to expand argument dir '%s': %v", ts.filename, err)
		}
		var url string
		// Compress if compress parameter is true
		if ts.compress {
			// Create buffer for the compressed data
			var buf bytes.Buffer
			// Create the archive and write the output to the "out" Writer
			buf, err = createTarArchive(path, buf)
			if err != nil {
				return fmt.Errorf("error creating an archive: %v", err)
			}
			fmt.Println("Tar archive created successfully")
			path = path + ".tar.gz"
			filename = filename + ".tar.gz"
			// Upload file
			url, err = ts.upload(path, filename, buf.Bytes())
			if err != nil {
				return fmt.Errorf("could not upload the file: %v", err)
			}
		} else {
			// Read the file that should be uploaded
			bodyBytes, err := ioutil.ReadFile(path)
			if err != nil {
				return fmt.Errorf("could not read the file: %v", err)
			}
			// Upload file
			url, err = ts.upload(path, filename, bodyBytes)
			if err != nil {
				return fmt.Errorf("could not upload the file: %v", err)
			}
		}
		// Emit URLtoReport event to get the url into the report
		if ts.emitURLtoReport {
			if err := emitEvent(ctx, EventURLtoReport, eventURLtoReportPayload{Msg: url}, target, ev); err != nil {
				return fmt.Errorf("failed to emit event: %v", err)
			}
		}
		return nil
	}
	return teststeps.ForEachTarget(Name, ctx, ch, f)
}

// Retrieve all the parameters defines through the jobDesc
func (ts *FileUpload) validateAndPopulate(params test.TestStepParameters) error {
	// Retrieving parameter as json Raw.Message
	// validate path and filename
	ts.path = params.GetOne("path")
	if ts.path.IsEmpty() {
		return fmt.Errorf("invalid or missing 'path' parameter, must be exactly one string")
	}
	ts.filename = params.GetOne("filename")
	if ts.filename.IsEmpty() {
		return fmt.Errorf("invalid or missing 'path' parameter, must be exactly one string")
	}

	// Retrieving parameter as string
	// validate s3region
	param := params.GetOne("s3region")
	if param.IsEmpty() {
		return fmt.Errorf("missing 's3region' parameter, must be exactly one string")
	}
	ts.s3region = param.String()
	// validate s3bucket
	param = params.GetOne("s3bucket")
	if param.IsEmpty() {
		return fmt.Errorf("missing 's3bucket' parameter, must be exactly one string")
	}
	ts.s3bucket = param.String()
	// validate s3path
	param = params.GetOne("s3path")
	if param.IsEmpty() {
		return fmt.Errorf("missing 's3path' parameter, must be exactly one string")
	}
	ts.s3path = param.String()
	// retrieve s3credfile
	ts.s3credfile = params.GetOne("s3credfile").String()
	// retrieve s3credprofile
	ts.s3credprofile = params.GetOne("s3credprofile").String()

	// Retrieving parameter as bool
	// validate emit_urltoreport
	param = params.GetOne("emit_urltoreport")
	if !param.IsEmpty() {
		v, err := strconv.ParseBool(param.String())
		if err != nil {
			return fmt.Errorf("invalid non-boolean `emit_urltoreport` parameter: %v", err)
		}
		ts.emitURLtoReport = v
	}
	// validate compress
	param = params.GetOne("compress")
	if !param.IsEmpty() {
		v, err := strconv.ParseBool(param.String())
		if err != nil {
			return fmt.Errorf("invalid non-boolean `emit_urltoreport` parameter: %v", err)
		}
		ts.compress = v
	}
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *FileUpload) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	return nil
}

// New initializes and returns a new awsFileUpload test step.
func New() test.TestStep {
	return &FileUpload{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}

// createTarArchive creates compressed data writer and invokes addFileToArchive
func createTarArchive(file string, buf bytes.Buffer) (bytes.Buffer, error) {
	// Create gzip and tar writers
	gzwriter := gzip.NewWriter(&buf)
	defer gzwriter.Close()
	tarwriter := tar.NewWriter(gzwriter)
	defer tarwriter.Close()
	// Write file into tar archive
	err := addFileToArchive(tarwriter, file)
	if err != nil {
		return buf, err
	}
	return buf, nil
}

// addFileToArchive takes the data and writes it into the tar archive
func addFileToArchive(tarwriter *tar.Writer, filename string) error {
	// Open the file which shall be written into the tar archive
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	// Retrieve the file stats
	info, err := file.Stat()
	if err != nil {
		return err
	}
	// Create a tar header from the file stats
	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return err
	}
	header.Name = filename
	// Write file header to the tar archive
	err = tarwriter.WriteHeader(header)
	if err != nil {
		return err
	}
	// Write the file into the tar archive
	_, err = io.Copy(tarwriter, file)
	if err != nil {
		return err
	}
	return nil
}

// Upload the file that is specified in the JobDescritor
func (ts *FileUpload) upload(path string, filename string, gzbuffer []byte) (string, error) {
	// Create a single AWS session (we can re use this if we're uploading many files)
	s, err := session.NewSession(&aws.Config{Region: aws.String(ts.s3region),
		Credentials: credentials.NewSharedCredentials(
			"", // your credential file path (default if empty)
			"", // profile name (default if empty)
		)})
	if err != nil {
		return "Could not open a new session.", err
	}
	currentTime := time.Now()
	// Config settings: this is where you choose the bucket, filename, content-type etc.
	// of the file you're uploading.
	fileName := fmt.Sprintf("%s/%s_%s", ts.s3path, currentTime.Format("20060102_150405"), filename)

	// Uploading the file
	_, err = s3.New(s).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(ts.s3bucket),
		Key:                  aws.String(fileName),
		ACL:                  aws.String("public-read"),
		Body:                 bytes.NewReader(gzbuffer),
		ContentLength:        aws.Int64(int64(len(gzbuffer))),
		ContentType:          aws.String(http.DetectContentType(gzbuffer)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
	})
	if err != nil {
		return "", err
	} else {
		fmt.Printf("Pushed the file to S3 Bucket! \n")
	}
	// Create download link for public ACL
	url := "https://" + ts.s3bucket + ".s3.amazonaws.com/" + fileName
	return url, err
}
