// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration_storage

package test

import (
	"bytes"
	"text/template"
)

var jobDescriptorTemplate = template.Must(template.New("jobDescriptor").Parse(`
{
    "JobName": "test job",
    "Reporting": {
        "FinalReporters": [
            {
                "Name": "noop"
            }
        ],
        "RunReporters": [
            {
                "Name": "TargetSuccess",
                "Parameters": {
                    "SuccessExpression": "=80%"
                }
            },
            {
                "Name": "Noop"
            }
        ]
    },
    "RunInterval": "3s",
    "Runs": 3,
    "Tags": [
        "test",
        "csv"
    ],
    "TestDescriptors": [
        {
            "TargetManagerAcquireParameters": {
                "Targets": [
                    {
                        "ID": "1234",
                        "Name": "example.org"
                    }
                ]
            },
            "TargetManagerName": "TargetList",
            "TargetManagerReleaseParameters": {},
            "TestFetcherFetchParameters": {
                "Steps": [ {{ . }} ],
                "TestName": "Literal test"
            },
            "TestFetcherName": "literal"
        }
    ]
} 
`))

var testSerializedTemplate = template.Must(template.New("testSerialized").Parse(`[[ {{ . }} ]]`))

func descriptorMust(data string) string {
	var buf bytes.Buffer
	if err := jobDescriptorTemplate.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.String()
}

func testSerializedMust(data string) string {
	var buf bytes.Buffer
	if err := testSerializedTemplate.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.String()
}

var steps = `
{
   "Name":"cmd",
   "Label":"some label",
   "Parameters":{
      "args":[
	 "Title={{ Title .Name }}, ToUpper={{ ToUpper .Name }}"
      ],
      "executable":[
	 "echo"
      ]
   }
}
`

var jobDescriptor = descriptorMust(steps)
var testSerialized = testSerializedMust(steps)
