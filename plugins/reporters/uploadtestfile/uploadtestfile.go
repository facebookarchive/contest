package uploadtestfile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/job"
)

// Define some constants for uploading things into a S3 bucket
const (
	S3_REGION = "" // your aws server region
	S3_BUCKET = "" // your aws bucket
	S3_PATH   = "" // your path where to upload the file
)

// Name defines the name of the reporter used within the plugin registry
var Name = "uploadtestfile"

// UploadTestFile is a reporter that can upload testdata to e.g. a S3 Bucket.
type UploadTestFile struct{}

// Parameters contains the parameter necessary for the final reporter to upload the file
type Parameter struct {
	Path     string
	Filename string
}

// ValidateRunParameters validates the parameter for the run reporter
func (n *UploadTestFile) ValidateRunParameters(params []byte) (interface{}, error) {
	// Unmarshal the params bytes into the RunParameter struct
	var rp Parameter
	if err := json.Unmarshal(params, &rp); err != nil {
		return nil, err
	}
	// Check if the file exists at all
	_, err := os.Stat(rp.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("the file that shall be uploaded is not existent: %w", err)
	}
	// Return the RunParameter
	return rp, nil
}

// ValidateFinalParameters validates the parameter for the final reporter
func (n *UploadTestFile) ValidateFinalParameters(params []byte) (interface{}, error) {
	// Unmarshal the params bytes into the FinalParameter struct
	var fp Parameter
	if err := json.Unmarshal(params, &fp); err != nil {
		return nil, err
	}
	// Check if the file exists at all
	_, err := os.Stat(fp.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("the file that shall be uploaded is not existent: %w", err)
	}
	// Return the FinalParameter
	return fp, nil
}

// Name returns the Name of the reporter
func (n *UploadTestFile) Name() string {
	return Name
}

// RunReport calculates the report to be associated with a job run.
func (n *UploadTestFile) RunReport(ctx context.Context, parameters interface{}, runStatus *job.RunStatus, ev testevent.Fetcher) (bool, interface{}, error) {
	// Retrieve the RunParameters
	RunParameter, ok := parameters.(Parameter)
	if !ok {
		return false, nil, fmt.Errorf("report parameter should be of type UploadTestFileParameters")
	}
	// Upload file
	url, err := upload(RunParameter)
	if err != nil {
		return false, url, err
	}
	return true, url, err
}

// FinalReport calculates the final report to be associated to a job.
func (n *UploadTestFile) FinalReport(ctx context.Context, parameters interface{}, runStatuses []job.RunStatus, ev testevent.Fetcher) (bool, interface{}, error) {
	// Retrieve the FinalParameters
	FinalParameter, ok := parameters.(Parameter)
	if !ok {
		return false, nil, fmt.Errorf("report parameter should be of type UploadTestFileParameters")
	}
	// Upload file
	url, err := upload(FinalParameter)
	if err != nil {
		return false, url, err
	}
	return true, url, err
}

// New builds a new TargetSuccessReporter
func New() job.Reporter {
	return &UploadTestFile{}
}

// Load returns the name and factory which are needed to register the Reporter
func Load() (string, job.ReporterFactory) {
	return Name, New
}

// Upload the file that is specified in the JobDescritor
func upload(parameter Parameter) (string, error) {
	// Read the file that should be uploaded
	bodyBytes, err := ioutil.ReadFile(parameter.Path)
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
	fileName := fmt.Sprintf("%s/%s_%s", S3_PATH, currentTime.Format("20060102_150405"), parameter.Filename)

	// Uploading the file
	_, err = s3.New(s).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(S3_BUCKET),
		Key:                  aws.String(fileName),
		ACL:                  aws.String("private"),
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
