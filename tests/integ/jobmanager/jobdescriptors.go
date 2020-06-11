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
                "label": "noop_label",
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
               "label": "slowecho_label",
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
               "label": "fail_label",
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
               "label": "crash_label",
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
               "label": "noreturn_label",
               "parameters": {}
           }
       ],
       "TestName": "IntegrationTest: noreturn"
   }`)

var jobDescriptorNoLabel = descriptorMust(`
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "noop",
               "parameters": {}
           }
       ],
       "TestName": "IntegrationTest: no_label"
   }`)

var jobDescriptorLabelDuplication = descriptorMust(`
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "noop",
               "label": "some_label_here",
               "parameters": {}
           },
           {
               "name": "noreturn",
               "label": "some_label_here",
               "parameters": {}
           }
       ],
       "TestName": "IntegrationTest: label_duplication"
   }`)

var jobDescriptorNullStep = descriptorMust(`
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "noop",
               "label": "some_label_here",
               "parameters": {}
           },
           null
       ],
       "TestName": "IntegrationTest: null TestStep"
   }`)

var jobDescriptorNullTest = `
{
    "JobName": "test job",
    "Tags": [
        "integration_testing"
    ],
    "TestDescriptors": [
        null
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
`
