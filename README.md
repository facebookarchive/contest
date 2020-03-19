# ConTest
[![Build Status](https://travis-ci.com/facebookincubator/contest.svg?branch=master)](https://travis-ci.com/facebookincubator/contest)
[![codecov](https://codecov.io/gh/facebookincubator/contest/branch/master/graph/badge.svg)](https://codecov.io/gh/facebookincubator/contest)
[![Go Report Card](https://goreportcard.com/badge/github.com/facebookincubator/contest)](https://goreportcard.com/report/github.com/facebookincubator/contest)

ConTest is a framework for system testing that aims at covering multiple use cases:
* system firmware testing
* provisioning testing
* system regressions
* microbenchmarks

and more. It provides a modular and pluggable set of interfaces to define your own methods to:
* fetch the information about which systems to run your test on
* describe and validate the initial state for the test systems, for a test to be able to run
* describe the type of tests to run (test definitions)
* implement system-specific actions (e.g. how to reboot it) and measurements (e.g. did the host boot into the OS?)
* maintain test state for each system on a customizable persistent storage
* describe and validate the final state for the test machines, for a test to be considered successful
* log the test progress and the final results, and allowing multiple backends


## Examples

You can look at the tools implemented under [cmds/*](cmds/) for usage examples.

## Requirements

ConTest is developed on Linux. It may work on other platforms, but it is only being actively tested on Linux.
You will also need a recent version of Go (we recommend at least Go 1.12 at the
moment).


## Quick Start

We offer a few Docker files to help setup ConTest. The fastest way to 
have a ConTest instance up and running is to bring up the server and MySQL
containers via the docker-compose configuration: just run
`docker-compose up --build` from the root of the source tree.

ConTest and MySQL containers can be also orchestrated separately:

* [docker/mysql](docker/mysql) will configure and bring up a MySQL
  instance with a pre-populated ConTest database that you can use with a local
  instance. Just run `docker-compose mysql up` from the root of the source tree.
* [docker/contest](docker/contest) supports running ConTest as standalone instance
as explained at the beginning of this section and also supports integration tests.
For a full test run please see `run_tests.sh`.


## Building ConTest

The recommended procedure is to download the ConTest framework via `go get`:

```
go get -u github.com/facebookincubator/contest
```
After successfully running this command, the ConTest framework will be available in your `GOPATH`, and you can start building your custom system testing infrastructure on top of it.

You can also have a look at the sample `contest` program:
```
go get -u github.com/facebookincubator/contest/cmds/contest
```

This will download and build the `contest` sample program, and will add it to your Go binary path.

## Running the sample server

There is a sample server under [cmds/contest](cmds/contest). All it does is to
register various plugins and user functions, and running ConTest's job manager
with a specific API listener.
You may want to adapt it to your needs by removing unnecessary plugins and
adding your custom ones, if necessary.
Additionally, the sample server uses the HTTP API listener. You may want to use
a different one or build your own.

After building the sample server as explained in the [Building
ConTest](#building-contest) section, run it with no arguments:
```
$ contest
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering target manager csvfiletargetmanager
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering target manager targetlist
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test fetcher uri
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test fetcher literal
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test step echo
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test step slowecho
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test step example
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test step cmd
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test step sshcmd
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test step randecho
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering test step terminalexpect
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering reporter targetsuccess
[2020-02-10T20:37:10Z]  INFO pkg/pluginregistry: Registering reporter noop
[2020-02-10T20:37:10Z]  INFO contest: Using database URI: contest:contest@tcp(localhost:3306)/contest?parseTime=true
[2020-02-10T20:37:10Z]  INFO contest: JobManager &{jobs:map[] jobRunner:0xc000021b10 jobsMu:{state:0 sema:0} jobsWg:{noCopy:{} state1:[0 0 0]} jobRequestManager:{JobRequestEmitter:{} JobRequestFetcher:{}} jobReportManager:{JobReportEmitter:{} JobReportFetcher:{}} frameworkEvManager:{FrameworkEventEmitter:{} FrameworkEventFetcher:{}} testEvManager:{} apiListener:0xd0dbc0 apiCancel:0xc000030660 pluginRegistry:0xc000072480}
[2020-02-10T20:37:10Z]  INFO listeners/httplistener: Started HTTP API listener on :8080
```

The server is informing us that it started the HTTP API listener on port 8080, after registering various types of plugins: target managers, test fetchers, test steps, and reporters.

ConTest also requires a database to store its state, events and other data.
The schema is defined under [docker/mysql/initdb.sql](docker/mysql/initdb.sql)
so you can create your own. We provide a docker image to bring up a database, so
you can use it with the sample server. Just run
```
docker-compose up mysql
```

from the top directory of ConTest's source code.

Once the database is up, it will possible to submit test jobs through the client,
as shown in the next section.

### Submitting jobs to the sample server

ConTest has no official CLI, because every user is different. However we provide
a sample CLI based on cleartext HTTP that will communicate to the HTTP API. All
you need to do is building the sample ConTest server as described above, and run
the CLI tool under [cmds/clients/contestcli-http](cmds/clients/contestcli-http).
The tool is very simple:
```
$ ./contestcli-http -h
Usage of contestcli-http:

  contestcli-http [args] command

command: start, stop, status, retry, version
  start
        start a new job using the job description passed via stdin
  stop int
        stop a job by job ID
  status int
        get the status of a job by job ID
  retry int
        retry a job by job ID
  version
        request the API version to the server

args:
  -addr string
    	ConTest server [scheme://]host:port[/basepath] to connect to (default "http://localhost:8080")
  -r string
    	Identifier of the requestor of the API call (default "contestcli-http")
exit status 2
```

To run a job, just feed a [job descriptor](#job-descriptors) to the client's `start` command, e.g. using
[start-literal.json](cmds/clients/contestcli-http/start-literal.json):

```
$ ./contestcli-http start < start-literal.json

[..output showing the job descriptor...]

The server responded with status 200 OK
{
  "ServerID": "barberio-ubuntu-9QCS5S2",
  "Type": "ResponseTypeStart",
  "Data": {
    "JobID": 12
  },
  "Error": null
}
```

Then we can get the status of the job using the `status` command and the job ID returned by the `start` request:
```
$ go run . status 12 | jq
Requesting URL http://localhost:8080/status with requestor ID 'contestcli-http'
  with params:
    jobID: [32]
    requestor: [contestcli-http]

The server responded with status 200 OK{
  "ServerID": "barberio-ubuntu-9QCS5S2",
  "Type": "ResponseTypeStatus",
  "Data": {
    "Status": {
      "EndTime": "0001-01-01T00:00:00Z",
      "JobReport": {
        "FinalReports": [
          {
            "Data": "I did nothing at the end, all good",
            "ReportTime": "2020-02-10T20:39:54Z",
            "Success": true
          }
        ],
        "JobID": 32,
        "RunReports": [
          [
            {
              "Data": {
                "AchievedSuccess": "100.00%",
                "DesiredSuccess": ">80.00%",
                "Message": "All tests pass success criteria: 100.00% > 80.00%"
              },
              "ReportTime": "2020-02-10T20:39:48Z",
              "Success": true
            },
            {
              "Data": "I did nothing on run #1, all good",
              "ReportTime": "2020-02-10T20:39:48Z",
              "Success": true
            }
          ],
          [
            {
              "Data": {
                "AchievedSuccess": "100.00%",
                "DesiredSuccess": ">80.00%",
                "Message": "All tests pass success criteria: 100.00% > 80.00%"
              },
              "ReportTime": "2020-02-10T20:39:51Z",
              "Success": true
            },
            {
              "Data": "I did nothing on run #2, all good",
              "ReportTime": "2020-02-10T20:39:51Z",
              "Success": true
            }
          ],
          [
            {
              "Data": {
                "AchievedSuccess": "100.00%",
                "DesiredSuccess": ">80.00%",
                "Message": "All tests pass success criteria: 100.00% > 80.00%"
              },
              "ReportTime": "2020-02-10T20:39:54Z",
              "Success": true
            },
            {
              "Data": "I did nothing on run #3, all good",
              "ReportTime": "2020-02-10T20:39:54Z",
              "Success": true
            }
          ]
        ]
      },
      "Name": "test job",
      "StartTime": "2020-02-10T20:39:48Z",
      "State": "",
      "TestStatus": [
        {
          "TestName": "Literal test",
          "TestStepStatus": [
            {
              "Events": [],
              "TargetStatus": [
                {
                  "Error": null,
                  "InTime": "2020-02-10T20:39:54Z",
                  "OutTime": "2020-02-10T20:39:54Z",
                  "Target": {
                    "FQDN": "",
                    "ID": "1234",
                    "Name": "example.org"
                  }
                }
              ],
              "TestStepLabel": "cmd",
              "TestStepName": "Cmd"
            }
          ]
        }
      ]
    }
  },
  "Error": null
}
```

## How does ConTest work

ConTest is a framework, not a program. You can use the framework to create your own system testing infrastructure on top of it.
We also provide a few sample programs under the [cmds/*](cmds/) directory that you can look at.

### Interfaces and plugins

ConTest is heavily based on interfaces and plugins. *Interfaces* are contracts
between the framework and its components, while *plugins* are implementations of
such interfaces.

From a high level perspective, the framework is modelled as a sequence of
stages, starting from the API listener until the reporting (see the
[design document](#design-document) section).

Each stage is defined by an interface, and can be implemented with a plugin. For
example, the API listener defines [an interface](/pkg/api/listener.go) that
requires a `Serve` method. The [http listener plugin](/plugins/listeners/http) implements such
method, and exposes various API operations like start or retry, and provides an HTTP endpoint to
communicate with the internal API. Of course you can implement your own plugin
to use a different protocol. It just has to respect the Listener interface and
do something useful.

Similarly, test steps are defined by the test step interface (see
[pkg/test/step.go](/pkg/test/step.go)). A plugin simply needs to implement such
interface and respect a few basic rules as defined in the developer documentation
(TODO). See for example the [sshcmd](/plugins/teststeps/sshcmd) plugin.

ConTest offers various plugins out of the box, which should be sufficient
for many use cases, but if you need more feel free to contribute with a pull
request, or to open an issue for a feature request. We are open to contributions
that are useful to the community!

### Job descriptors

One of the main concepts in ConTest is the job descriptor. It describes how a test job will behave on each device under test.
A job descriptor has to declare:
* how to get the [targets](#targets) to run the test jobs on
* how to get the [test descriptors](#test-descriptors) that will be executed for each target
* how to report success or failure

An example of a job descriptor that runs a command over SSH to a target host
is available at [cmds/clients/http/start.json](cmd/clients/http/start.json), reported below and
commented for clarity:
```
{
    // The job name. This can be used to search and correlate job events.
    "JobName": "test job",
    // number of times a job should be run. 0 or a negative number means
    // "run forever". Also see TestDescriptors below.
    "Runs": 5,
    // How much time to wait between runs. Any string suitable for
    // [time.ParseDuration](https://golang.org/pkg/time/#ParseDuration) will work.
    // Also see TestDescriptors below.
    "RunInterval": "5s",
    // Tags can be used for search and aggregation. Currently not used.
    "Tags": ["test", "csv"],
    // A list of test descriptors that contain all the information to run a
    // job. At least one test descriptor is required (like in the example below),
    // but there is virtually no limit to how many descriptors a user can specify.
    // Each test associated to a descriptor is run sequentially. This is
    // intentional, and parallelism can be achieved in other ways.
    //
    // Note: the Runs parameter above is the number of times that all the test
    // descriptors are run in total, sequentially.
    // Note: the RunInterval parameter above is the time the framework will wait
    // between multiple runs of a list of test descriptors.
    "TestDescriptors": [
        {
            // The name of the target manager that will be called to get a list
            // of targets to run tests against. The plugins must be registered
            // (see cmds/contest/main.go). Various
            // target manager plugins are already available in
            // [plugins/targetmanagers](plugins/targetmanagers).
            "TargetManagerName": "CSVFileTargetManager",
            // parameters to be passed to the target manager. This is dependent
            // on the actual plugin.
            "TargetManagerAcquireParameters": {
                // the URI for the CSV file that contains a list of targets in
                the format "ip,hostname". This is intentionally very simple.
                "FileURI": "hosts02.csv",
                // The minimum number of targets needed for the test. If we
                // don't get at least this number of devices, the job will fail.
                "MinNumberDevices": 10,
                // The maximum number of targets that we need. The plugin should
                // try to always return this number of targets, unless fewer are
                // available.
                "MaxNumberDevices": 20,
                // the host name prefixes used to filter the devices from the
                // CSV file. For example, compute001.facebook.com will match in
                // the example below, but web001.facebook.com will not.
                "HostPrefixes": [
                    "compute",
                    "storage"
                ]
            },
            // parameters that are passed to the target manager when it is asked
            // to release the targets at the end of a test. This is also depending
            // on the plugin. In this case there is no parameter for releasing
            // the targets.
            "TargetManagerReleaseParameters": {
            },
            // The name of the plugin used to fetch the test definitions. The
            // test fetcher plugins must be registered in main.go just like we
            // do for target managers (see above).
            // Tests can be stored somewhere else, so the concept of fetching
            // tests.
            // Here we are using the URI plugin, which allows specifying file
            // paths that are local to the ConTest server (note: not local to the
            // client), but also http:// and https:// in case you prefer to
            // store files on a remote HTTP endpoint. New plugins can be
            // developed to implement more complex fetching logic.
            //
            // Note: if you prefer to specify the test steps inline in the test
            // descriptor you can do that using the "literal" test fetcher plugin.
            // See cmds/clients/http/start-literal.json for an example.
            "TestFetcherName": "URI",
            // Parameters to be passed to the test fetcher. This is dependent on
            // the plugin.
            "TestFetcherFetchParameters": {
                // Test name used for searching and correlation.
                "TestName": "My Test Name",
                // URI of the JSON containing the actual test step definitions.
                // This is a file that is local to the ConTest server. You can
                // also specify an absolute path, or use any of file://, http://
                // and https:// schemes.
                //
                // Note: in the section below we will show a commented example
                // of test step definitions.
                "URI": "test_samples/randecho.json"
            }
        }
    ],
    // The reporting section is divided in two parts: run reporters and final
    // reporters. At least one reporter must be specified, of any type, or the
    // job will be rejected.
    "Reporting": {
        // Run reporters are plugins that are executed at the end of each test
        // run.
        // They are meant to report results of individual runs, as opposed to
        // the full job inclusive of all of its runs. The number of runs is
        // specified above in the job descriptor.
        //
        // A reporter plugin can implement run reporting, or final reporting, or
        // both. The two implementations would normally be different and
        // independent.
        "RunReporters": [
            {
                // TargetSuccess is the name of the reporter plugin. Every
                // plugin has its own specific arguments, so there is no common
                // format. The plugin will know how to parse the JSON
                // subdocument.
                // In this example, TargetSuccess will determine the success of
                // the job based on how many devices could make it until the end
                // of the test. Both percentage and target numbers can be
                // specified.
                "Name": "TargetSuccess",
                "Parameters": {
                    "SuccessExpression": ">80%"
                }
            },
            {
                // This is another run reporter. It will be executed after the
                // above, and it will have its own concept of "success". Every
                // reporter carries its own success indicator, so there is no
                // overall "this job was successful", rather "this reporter was
                // successful". For example, one reporter may succeed if enough
                // targets made it until the end of the tests, whike another
                // reporter may measure performance regressions, and fail. The
                // two results would be independent from each other.
                "Name": "Noop"
            }
        ],
        // Final reporters instead will be executed at the very end, and will
        // normally be used to report the results of the whole set of runs.
        //
        // If the job is running in continuous mode (i.e. Runs is 0), the final
        // reporters will only be run if the job is cancelled.
        //
        // In the example below there is only one reporter, but it's possible to
        // specify more, just like for run reporters.
        "FinalReporters": [
            {
                "Name": "noop"
            }
        ]
    }

    // The name of the reporter plugin. This is used to decide and report
    whether a job was successful or not.
    "ReporterName": "TargetSuccess",
    // The parameters to a reporter plugin. This is is plugin-dependant too.
    "ReporterParameters": {
        // The TargetSuccess plugin allows for simple expressions like ">=50%"
        // or ">10", meaning respectively "at least 50% of the targets", and
        // "more than 10 targets". This is a common reporting scenario, but we
        // expect users to implement their own custom reporting logic.
        "SuccessExpression": ">50%"
   }
}
```

### Test fetchers

Test fetchers are responsible for retrieving the test steps that we want to run
on each target.

Test steps are encoded encodes in JSON format. For example:
```
{
    "steps": [
        {
            "name": "cmd",
            "label": "some label",
            "parameters": {
                "executable": ["echo"],
                "args": ["Hello, world!"]
            }
        }
    ]
}
```

The above test steps describe a test run with a single step. Such step will
use the `cmd` plugin to execute the command `echo "Hello, world!"` on the
ConTest server.
If you want to add more test steps, just add more items to the `steps` list.

In the [job descriptors](#job-descriptors) paragraph we have shown an example of
using the `URI` test fetcher. The `URI` plugin lets you get your test steps
using an URI, e.g. "https://example.org/test/my-test-steps.json". This is
convenient when storing and reusing test steps in a remote repository or in a
local file, but one size does not fit all. You may want, for example, specify
all of your test steps inline. For the purpose we wrote the `literal` test
fetcher plugin, which allows you to specify the test steps inline in the job
descriptor. Just insert the content of your JSON file containing the test steps
into the job descriptor. For an example, see
[start-literal.json](cmds/clients/contestcli-http/start-literal.json).

TODO where are they stored
TODO square brackets

### Targets

Targets are a generic representation of the entity where test jobs are run on.

Every job is associated to a list of targets, and what actions (and how) are
executed on each target depends on the plugin.

Targets are currently defined with three properties:
* **Name**: a mnemonic name associated to the target. It could be its DNS short or
  fully-qualified name, or any other name that can be associated to it.
* **ID**: an identifier for the target. It should be unique, however this is not
  enforced by the framework. For example, an identifier could be the target's
  asset ID, a hardware fingerprint, but again, it can really be anything that
  makes sense to the user.
* **FQDN**: this field can be used to attribute a fully-qualified domain name to
  the target. The presence and the well-formedness of this field are not
  checked nor enforced.

The `Target` structure is defined in [pkg/target](pkg/target/target.go). Plugin
configurations can access the specific fields via Go templates, as explained in
more detail in the [Templates in test step arguments](#templates-in-plugin-configurations)
section. For example, to print a target's name and ID with the `cmd` plugin:

```
{
    "steps": [
        {
            "name": "cmd",
            "label": "some label",
            "parameters": {
                "executable": ["echo"],
                "args": ["Name is {{ .Name }} and ID is {{ .ID }}"]
            }
        }
    ]
}
```

This will be expanded and executed for every target in the test job.

### Templates in plugin configurations

Many plugins support Go templating in the test step definitions using
[text/template](https://golang.org/pkg/text/template/). This means that
it is possible to use functions and substitutions to configure a plugin's action
more dynamically. This is useful for example for steps that need target-specific
information, or when the plugin configuration needs some run-time or dynamic
manipulation.

The standard Go templating functions are available, and it is also possible to
register user functions.
The `Target` object is passed to the template as the root object.

For example, to instruct the `cmd` plugin to print the name of a target, the
following snippet can be used in the test step configuration. Note: the `cmd`
plugin executes a command on the ConTest server, not on the targets.

```
...
    {
        "name": "cmd",
        "label": "some label",
        "parameters: {
            "executable": ["echo"],
            "args": ["My name is {{ .Name }}"]
        }"
    }
...
```

Let's dissect the above.

* the "name" directive tells ConTest to run the "cmd" plugin. This plugin will
  be run on every target in the test job. It's the plugin's responsibility to
  know what to do with the targets: the framework simply routes them to each
  step's plugin.
* the "parameters" map contains the arguments to pass to the plugin.
* the "executable" parameter specifies what to execute
* the "args" parameter is the list of arguments to pass to the program execution

Note that "args" contains only one argument, and this argument uses the Go
templating syntax. `.Name` expands to the value contained in `Target.Name`,
since the target is the root object passed to the template. This means that you
can also use `.ID` or `.FQDN` if you want to access other members of the target
structure.
After the name expansion is done, the resulting string will be unique per
target, and ConTest will execute the "echo" command with this customized output
for each target.

Substitution is not the only thing one can do with templates. Functions are also
available, for example to further manipulate our configuration data. All of the
built-in functions from `text/template` are available. To slice a string one can
write:

```
...
    {
        "name": "cmd",
        "label": "some label",
        "parameters: {
            "executable": ["echo"],
            "args": ["The first four letters of my name are {{ slice .Name 0 4 }}"]
        }"
    }
...
```

ConTest defines additional built-in functions in
[pkg/test](pkg/test/functions.go). For example, to print the target name after
capitalzing its letter, one can write:

```
...
    {
        "name": "cmd",
        "label": "some label",
        "parameters: {
            "executable": ["echo"],
            "args": ["My capitalized name is {{ ToUpper .Name }}"]
        }"
    }
...
```

ConTest also allows the user to register their own functions with
`test.RegisterFunction` from [pkg/test/functions.go](pkg/test/functions.go).
See [cmds/contest/main.go](cmds/contest/main.go) for an example of how to use
it.

Custom functions are useful to enable more complex use cases. For example, if we
want to execute a command on a jump host that depends on the target name, we can
write a custom template function to get this information. There is no limitation
on how to implement it: it can be simple string substitution, or it can be
retrieved from a backend service requiring authentication.
For example, imagine that we wrote a custom template function called `jumphost`,
that receives the target name as input and returns the jump host (or an error),
we can use it as follows. Note that the "sshcmd" plugin runs a command on a
remote host.

```
...
    {
        "name": "sshcmd",
        "label": "some label...",
        "parameters: {
            "user": "contest",
            "host": "{{ jumphost .Name }}",
            "private_key_file": "/path/to/id_dsa",
            "executable": ["echo"],
            "args": ["ConTest was here"]
        }"
    }
...
```


Go templates allow for more powerful actions, like loops and conditionals, so we
recommend reading the [text/template](https://golang.org/pkg/text/template/)
documentation.

Limitations:
* currently every plugin must explicitly call template expansion. We plan to
  make this free and automatic for every plugin in the future.


## Join the ConTest community

* Website: https://github.com/facebookincubator/contest . Please use issues and
  pull requests!
* IM: we are on the #ConTest channel on the OSF (Open Source Firmware) Slack team. Get your invite on http://u-root.org if you're not there already!
* See the CONTRIBUTING file for how to help out.

## License

ConTest is MIT licensed, as found in the LICENSE file.
