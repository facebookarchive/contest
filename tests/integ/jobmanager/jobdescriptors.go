// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// +build integration integration_storage

package test

var jobDescriptorPre = `
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
`

var jobDescriptorPost = `
      "ReporterParameters": {
        "SuccessExpression": ">0%"
      }
    }
  ]
}
`

var jobDescriptorNoop = jobDescriptorPre + `
            "TestFetcherFetchParameters": {
                "Steps": [
                    {
                        "name": "noop",
                        "parameters": {}
                    }
                ],
                "TestName": "IntegrationTest"
            },` + jobDescriptorPost

var jobDescriptorSlowecho = jobDescriptorPre + `
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
              "TestName": "IntegrationTest"
          },` + jobDescriptorPost

var jobDescriptorFailure = jobDescriptorPre + `
          "TestFetcherFetchParameters": {
              "Steps": [
                  {
                      "name": "fail",
                      "parameters": {}
                  }
              ],
              "TestName": "IntegrationTest"
          },` + jobDescriptorPost

var jobDescriptorCrash = jobDescriptorPre + `
          "TestFetcherFetchParameters": {
              "Steps": [
                  {
                      "name": "crash",
                      "parameters": {}
                  }
              ],
              "TestName": "IntegrationTest"
          },` + jobDescriptorPost

var jobDescriptorHang = jobDescriptorPre + `
          "TestFetcherFetchParameters": {
              "Steps": [
                  {
                      "name": "noreturn",
                      "parameters": {}
                  }
              ],
              "TestName": "IntegrationTest"
          },` + jobDescriptorPost
