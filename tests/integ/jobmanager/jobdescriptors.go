// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

import (
	"bytes"
	"text/template"
)

var jobDescriptorTemplate = template.Must(template.New("jobDescriptor").Parse(`
{
    "JobName": "test job",
    "ReporterName": "TargetSuccess",
    "ReporterParameters": {
        "SuccessExpression": ">10%"
    },
    "RunInterval": "5s",
    "Runs": 1,
    "Tags": [
        "integration_testing"
    ],
    "TestDescriptors": [
        {
            "TargetManagerName": "TargetList",
            "TargetManagerAcquireParameters": {
                "Targets": [
                    {
                        "ID": "id1",
                        "Name": "hostname1.example.com"

                    },
                    {
                        "ID": "id2",
                        "Name": "hostname2.example.com"
                    }
                ]
            },
            "TargetManagerReleaseParameters": {},
            "TestFetcherName": "literal",
            {{ . }}
        }
    ],
    "Reporting": {
        "RunReporters": [
            {
                "Name": "TargetSuccess",
                "Parameters": {
                    "SuccessExpression": ">0%"
                }
            }
        ]
    }
}
`))

func descriptorMust(data string) string {
	var buf bytes.Buffer
	if err := jobDescriptorTemplate.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.String()
}

var jobDescriptorNoop = descriptorMust(`
    "TestFetcherFetchParameters": {
        "Steps": [
            {
                "name": "noop",
                "parameters": {}
            }
        ],
        "TestName": "IntegrationTest: noop"
    }`)

var jobDescriptorSlowecho = descriptorMust(`
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "slowecho",
               "parameters": {
                 "sleep": ["5"],
                 "text": ["Hello world"]
               }
           }
       ],
       "TestName": "IntegrationTest: slow echo"
   }`)

var jobDescriptorFailure = descriptorMust(`
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "fail",
               "parameters": {}
           }
       ],
       "TestName": "IntegrationTest: fail"
   }`)

var jobDescriptorCrash = descriptorMust(`
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "crash",
               "parameters": {}
           }
       ],
       "TestName": "IntegrationTest: crash"
   }`)

var jobDescriptorHang = descriptorMust(`
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "noreturn",
               "parameters": {}
           }
       ],
       "TestName": "IntegrationTest: noreturn"
   }`)
