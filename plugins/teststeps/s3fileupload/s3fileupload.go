// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package s3fileupload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
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
var Name = "s3FileUpload"

// Event names for this plugin.
const (
	EventURL = event.Name("URL")
)

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{
	EventURL,
}

// FileUpload is used to retrieve all the parameter, the plugin needs.
type FileUpload struct {
	localPath     *test.Param // Path to file that shall be uploaded
	fileName      *test.Param // Filename to file that shall be uploaded
	s3Region      string      // AWS server region that shall be used
	s3Bucket      string      // AWS Bucket name
	s3Path        string      // Path where in the bucket to upload the file
	s3CredFile    string      // Path to the AWS credential file to authenticate the session
	s3CredProfile string      // Profile of the AWS credential file that shall be used
	compGzip      bool        // Bool that defines if the data should be compressed to gzip before upload
}

// Datatype for the emmiting the URL Event
type eventURLPayload struct {
	Msg string
}

// Name returns the plugin name.
func (ts FileUpload) Name() string {
	return Name
}

func emitEvent(ctx xcontext.Context, name event.Name, payload interface{}, tgt *target.Target, ev testevent.Emitter) error {
	payloadStr, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cannot encode payload for event '%s': %w", name, err)
	}
	rm := json.RawMessage(payloadStr)
	evData := testevent.Data{
		EventName: name,
		Target:    tgt,
		Payload:   &rm,
	}
	if err := ev.Emit(ctx, evData); err != nil {
		return fmt.Errorf("cannot emit event EventURL: %w", err)
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
		path, err := ts.localPath.Expand(target)
		if err != nil {
			return fmt.Errorf("failed to expand argument '%s': %v", ts.localPath, err)
		}
		filename, err := ts.fileName.Expand(target)
		if err != nil {
			return fmt.Errorf("failed to expand argument dir '%s': %v", ts.fileName, err)
		}
		// bodyBytes will contain the file data to upload it
		var bodyBytes []byte
		// Compress if compress parameter is true
		if ts.compGzip {
			// Create the archive and write the output to the "out" Writer
			buf, err := createTarArchive(path, ctx)
			if err != nil {
				return fmt.Errorf("error creating an archive: %v", err)
			}
			filename = filename + ".tar.gz"
			bodyBytes = buf.Bytes()
		} else {
			// Read the file that should be uploaded
			bodyBytes, err = os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("could not read the file: %v", err)
			}
		}
		// Upload file
		url, err := ts.upload(filename, bodyBytes, ctx)
		if err != nil {
			return fmt.Errorf("could not upload the file: %v", err)
		}
		// Emit URL event to get the url into the report
		if err := emitEvent(ctx, EventURL, eventURLPayload{Msg: url}, target, ev); err != nil {
			return fmt.Errorf("failed to emit event: %w", err)
		}
		return nil
	}
	return teststeps.ForEachTarget(Name, ctx, ch, f)
}

// Retrieve all the parameters defines through the jobDesc
func (ts *FileUpload) validateAndPopulate(params test.TestStepParameters) error {
	// Retrieving parameter as json Raw.Message
	// validate path and filename
	ts.localPath = params.GetOne("path")
	if ts.localPath.IsEmpty() {
		return fmt.Errorf("missing or empty 'path' parameter")
	}
	ts.fileName = params.GetOne("filename")
	if ts.fileName.IsEmpty() {
		return fmt.Errorf("missing or empty 'filename' parameter")
	}

	// Retrieving parameter as string
	// validate s3region
	param := params.GetOne("s3region")
	if param.IsEmpty() {
		return fmt.Errorf("missing or empty 's3region' parameter")
	}
	ts.s3Region = param.String()
	// validate s3bucket
	param = params.GetOne("s3bucket")
	if param.IsEmpty() {
		return fmt.Errorf("missing or empty 's3bucket' parameter")
	}
	ts.s3Bucket = param.String()
	// validate s3path
	param = params.GetOne("s3path")
	if param.IsEmpty() {
		return fmt.Errorf("missing or empty 's3path' parameter")
	}
	ts.s3Path = param.String()
	// retrieve s3credfile
	ts.s3CredFile = params.GetOne("s3credfile").String()
	// retrieve s3credprofile
	ts.s3CredProfile = params.GetOne("s3credprofile").String()

	// Retrieving parameter as bool
	// validate compress
	param = params.GetOne("compgzip")
	if !param.IsEmpty() {
		v, err := strconv.ParseBool(param.String())
		if err != nil {
			return fmt.Errorf("invalid non-boolean `compgzip` parameter: %v", err)
		}
		ts.compGzip = v
	}
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *FileUpload) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	return ts.validateAndPopulate(params)
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
func createTarArchive(file string, ctx xcontext.Context) (*bytes.Buffer, error) {
	// Create buffer for the compressed data
	var buf bytes.Buffer
	// Create gzip and tar writers
	gzwriter := gzip.NewWriter(&buf)
	defer gzwriter.Close()
	tarwriter := tar.NewWriter(gzwriter)
	defer tarwriter.Close()
	// Write file into tar archive
	err := addFileToArchive(tarwriter, file, ctx)
	if err != nil {
		return &buf, err
	}
	return &buf, nil
}

// addFileToArchive takes the data and writes it into the tar archive
func addFileToArchive(tarwriter *tar.Writer, filename string, ctx xcontext.Context) error {
	// Open the file which shall be written into the tar archive
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open '%s': %w", filename, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			ctx.Warnf("failed to close file '%s': %w", filename, err)
		}
	}()

	// Retrieve the file stats
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to retrieve stats of '%s': %w", filename, err)
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
func (ts *FileUpload) upload(filename string, data []byte, ctx xcontext.Context) (string, error) {

	// Create an AWS session
	s, err := session.NewSession(&aws.Config{Region: aws.String(ts.s3Region),
		Credentials: credentials.NewSharedCredentials(
			ts.s3CredFile,    // your credential file path (default if empty)
			ts.s3CredProfile, // profile name (default if empty)
		)})
	if err != nil {
		return "", fmt.Errorf("could not open a new session: %w", err)
	}

	// Creating an upload path where the file should be uploaded with a timestamp
	currentTime := time.Now()
	uploadPath := strings.Join([]string{ts.s3Path, currentTime.Format("20060102_150405")}, "/")
	uploadPath = strings.Join([]string{uploadPath, filename}, "_")

	// Uploading the file
	_, err = s3.New(s).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(ts.s3Bucket),
		Key:                  aws.String(uploadPath),
		ACL:                  aws.String("public-read"),
		Body:                 bytes.NewReader(data),
		ContentLength:        aws.Int64(int64(len(data))),
		ContentType:          aws.String(http.DetectContentType(data)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
	})
	if err != nil {
		return "", fmt.Errorf("could not upload the file: %w", err)
	} else {
		ctx.Infof("Pushed the file to S3 Bucket!")
	}
	// Create download link for public ACL
	url := strings.Join([]string{"https://", ts.s3Bucket, ".s3.amazonaws.com/", uploadPath}, "")
	return url, nil
}
