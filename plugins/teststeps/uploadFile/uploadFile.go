// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package uploadfile

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
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
	"github.com/prometheus/common/log"
)

// Define some constants for uploading things into a S3 bucket
const (
	S3_REGION = "" // your aws server region
	S3_BUCKET = "" // your aws bucket
	S3_PATH   = "" // your path where to upload the file
)

// Name is the name used to look this plugin up.
var Name = "UploadFile"

// Event names for this plugin.
const (
	EventCmdStdout = event.Name("CmdStdout")
)

// Events defines the events that a TestStep is allow to emit
var Events = []event.Name{
	EventCmdStdout,
}

// Cmd is used to run arbitrary commands as test steps.
type UploadFile struct {
	path       *test.Param // Path to file that shall be uploaded
	filename   *test.Param // Filename to file that shall be uploaded
	emitStdout bool        // Bool that defines that Stdout Event shall be triggered
	compress   bool        // Bool that defines if the data should be compressed before uploading it
}

// Datatype for the emmiting the Stdout Event
type eventCmdStdoutPayload struct {
	Msg string
}

// Name returns the plugin name.
func (ts UploadFile) Name() string {
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
		return fmt.Errorf("cannot emit event EventCmdStart: %v", err)
	}
	return nil
}

// Run executes the cmd step.
func (ts *UploadFile) Run(ctx xcontext.Context, ch test.TestStepChannels, params test.TestStepParameters,
	ev testevent.Emitter, resumeState json.RawMessage) (json.RawMessage, error) {

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
		// Compress if compress parameter is true
		if ts.compress {
			// Create output file
			newpath := path + ".tar.gz"
			tar_gz, err := os.Create(newpath)
			if err != nil {
				return fmt.Errorf("error writing archive: %w", err)
			}
			defer tar_gz.Close()
			// Create the archive and write the output to the "out" Writer
			err = createTarArchive(path, tar_gz)
			if err != nil {
				log.Fatalln("Error creating archive:", err)
			}
			fmt.Println("Tar archive created successfully")
			path = newpath
			filename = filename + ".tar.gz"
		}
		// Upload file
		url, err := upload(path, filename)
		fmt.Println(url)
		if err != nil {
			return err
		}
		if ts.emitStdout {
			log.Infof("Emitting stdout event")
			if err := emitEvent(ctx, EventCmdStdout, eventCmdStdoutPayload{Msg: url}, target, ev); err != nil {
				log.Warnf("Failed to emit event: %v", err)
			}
		}
		return nil
	}
	return teststeps.ForEachTarget(Name, ctx, ch, f)
}

// Retrieve all the parameters defines through the jobDesc
func (ts *UploadFile) validateAndPopulate(params test.TestStepParameters) error {
	// validate path and filename
	ts.path = params.GetOne("path")
	if ts.path.IsEmpty() {
		return errors.New("invalid or missing 'path' parameter, must be exactly one string")
	}
	ts.filename = params.GetOne("filename")
	if ts.filename.IsEmpty() {
		return errors.New("invalid or missing 'filename' parameter, must be exactly one string")
	}
	// validate emit_stdout
	emitStdoutParam := params.GetOne("emit_stdout")
	if !emitStdoutParam.IsEmpty() {
		v, err := strconv.ParseBool(emitStdoutParam.String())
		if err != nil {
			return fmt.Errorf("invalid non-boolean `emit_stdout` parameter: %v", err)
		}
		ts.emitStdout = v
	}
	// validate compress
	compressParam := params.GetOne("compress")
	if !compressParam.IsEmpty() {
		v, err := strconv.ParseBool(compressParam.String())
		if err != nil {
			return fmt.Errorf("invalid non-boolean `emit_stdout` parameter: %v", err)
		}
		ts.compress = v
	}
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *UploadFile) ValidateParameters(_ xcontext.Context, params test.TestStepParameters) error {
	if params.Get("path") == nil {
		return fmt.Errorf("could not validate parameter")
	}
	return nil
}

// New initializes and returns a new Cmd test step.
func New() test.TestStep {
	return &UploadFile{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}

func createTarArchive(file string, buf io.Writer) error {
	// Create gzip and tar writers
	gzwriter := gzip.NewWriter(buf)
	defer gzwriter.Close()
	tarwriter := tar.NewWriter(gzwriter)
	defer tarwriter.Close()
	// Write file into tar archive
	err := addFileToArchive(tarwriter, file)
	if err != nil {
		return err
	}
	return nil
}

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
func upload(path string, filename string) (string, error) {
	// Read the file that should be uploaded
	bodyBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return "Could not read the file.", err
	}

	// Create a single AWS session (we can re use this if we're uploading many files)
	s, err := session.NewSession(&aws.Config{Region: aws.String(S3_REGION),
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
	fileName := fmt.Sprintf("%s/%s_%s", S3_PATH, currentTime.Format("20060102_150405"), filename)

	// Uploading the file
	_, err = s3.New(s).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(S3_BUCKET),
		Key:                  aws.String(fileName),
		ACL:                  aws.String("public-read"),
		Body:                 bytes.NewReader(bodyBytes),
		ContentLength:        aws.Int64(int64(len(bodyBytes))),
		ContentType:          aws.String(http.DetectContentType(bodyBytes)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
	})
	if err != nil {
		return "Could no upload the file.", err
	} else {
		fmt.Printf("Pushed the file to S3 Bucket! \n")
	}
	// Create download link for public ACL
	url := "https://" + S3_BUCKET + ".s3.amazonaws.com/" + fileName

	return url, err
}
