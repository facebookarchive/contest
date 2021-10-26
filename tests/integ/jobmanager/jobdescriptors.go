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
    "Runs": {{ .Runs }},
    "RunInterval": "{{ .RunInterval }}",
    "Tags": [
        "integration_testing" {{ .ExtraTags }}
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
            {{ .Def }}
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

type templateData struct {
	Runs        int
	RunInterval string
	ExtraTags   string
	Def         string
}

func descriptorMust2(data *templateData) string {
	var buf bytes.Buffer
	if err := jobDescriptorTemplate.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.String()
}

func descriptorMust(def string) string {
	return descriptorMust2(&templateData{Runs: 1, RunInterval: "1s", Def: def})
}

var testStepsNoop = `
    "TestFetcherFetchParameters": {
        "Steps": [
            {
                "name": "noop",
                "label": "noop_label",
                "parameters": {}
            }
        ],
        "TestName": "IntegrationTest: noop"
    }`
var jobDescriptorNoop = descriptorMust(testStepsNoop)
var jobDescriptorNoop2 = descriptorMust2(
	&templateData{Runs: 1, RunInterval: "1s", Def: testStepsNoop, ExtraTags: `, "foo"`},
)

var jobDescriptorSlowEcho = descriptorMust(`
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "slowecho",
               "label": "slowecho_label",
               "parameters": {
                 "sleep": ["0.5"],
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
       "TestName": "TestTestStepLabelDuplication"
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
var jobDescriptorBadTag = descriptorMust2(&templateData{
	Runs:        2,
	RunInterval: "1s",
	ExtraTags:   `, "a bad one"`,
	Def: `
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "slowecho",
               "label": "slowecho_label",
               "parameters": {
                 "sleep": ["2"],
                 "text": ["Hello world"]
               }
           }
       ],
       "TestName": "IntegrationTest: slow echo"
   }`,
})

var jobDescriptorInternalTag = descriptorMust2(&templateData{
	Runs:        2,
	RunInterval: "1s",
	ExtraTags:   `, "_foo"`,
	Def: `
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "slowecho",
               "label": "slowecho_label",
               "parameters": {
                 "sleep": ["2"],
                 "text": ["Hello world"]
               }
           }
       ],
       "TestName": "IntegrationTest: slow echo"
   }`,
})

var jobDescriptorDuplicateTag = descriptorMust2(&templateData{
	Runs:        2,
	RunInterval: "1s",
	ExtraTags:   `, "qwe", "qwe"`,
	Def: `
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "slowecho",
               "label": "slowecho_label",
               "parameters": {
                 "sleep": ["2"],
                 "text": ["Hello world"]
               }
           }
       ],
       "TestName": "IntegrationTest: slow echo"
   }`,
})

var jobDescriptorSlowEcho2 = descriptorMust2(&templateData{
	Runs:        2,
	RunInterval: "0.5s",
	Def: `
   "TestFetcherFetchParameters": {
       "Steps": [
           {
               "name": "slowecho",
               "label": "Step 1",
               "parameters": {
                 "sleep": ["0.5"],
                 "text": ["Hello step 1"]
               }
           },
           {
               "name": "slowecho",
               "label": "Step 2",
               "parameters": {
                 "sleep": ["0"],
                 "text": ["Hello step 2"]
               }
           }
       ],
       "TestName": "IntegrationTest: resume"
   }`,
})

var jobDescriptorReadmeta = `
{
    "JobName": "test job",
    "Runs": 1,
    "RunInterval": "5s",
    "Tags": [
        "integration_testing"
    ],
    "TestDescriptors": [
        {
            "TargetManagerName": "readmeta",
            "TargetManagerAcquireParameters": {},
            "TargetManagerReleaseParameters": {},
            "TestFetcherName": "literal",
            "TestFetcherFetchParameters": {
                "Steps": [
                    {
                        "name": "readmeta",
                        "label": "readmeta_label",
                        "parameters": {}
                    }
                ],
                "TestName": "IntegrationTest: noop"
            }
        }
    ],
    "Reporting": {
        "RunReporters": [
            {
                "Name": "readmeta",
                "Parameters": {}
            }
        ],
        "FinalReporters": [
            {
                "Name": "readmeta",
                "Parameters": {}
            }
        ]
    }
}
`
