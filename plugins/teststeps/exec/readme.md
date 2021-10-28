# Exec plugin

The *exec* plugin runs a given binary or executable and determines whether the test passed based on criteria specified by options. The executables are run using a transport that abstracts the mechanism of launching the process.

## Parameters

Top-level parameter is called `bag` and it has a single element with the actual plugin parameter values.
- `bin`: specifies the `path` and `args` for the executable
- `transport`: specifies the protocol to use and options for that specific protocol
- `constraints`: currently just has `time_quota`, the maximum duration any local process can run in the context of the Contest server (more details below per transport). Value of 0 means infinite quota.
- `ocp_output` *(default: false)*: if this is true, the output of the process is parsed as per OCP Testing & Validation specification in order to decide whether it passed or not. When set to false, the test is considered passed if exit code was 0.

### Transport options

Proto **local**:
- this has no specific options, just runs the given binary on the same server as the Contest process. The binary must exist on that filesystem.
- the top level `time_quota` applies here and when it is exceeded, the Contest server kills the process

Of note is the fact that the Contest server binary must be built (or run) with tag `unsafe` for this to be available. By default a stub implementation is used that just errors out. The consideration here is that arbitrary code can be run on the Contest server machine using this transport therefore making the machine insecure.

Proto **ssh**:
- `host`: ip/hostname that has the ssh server to connect to
- `port` *(default: 22)*: ssh server port
- `user`: ssh user to use on connect
- `password` *(default: empty)*: ssh password to use; if empty, password auth is not considered
- `identity_file` (default: empty): ssh private key to use as identity; if empty, pubkey auth is not considered
- `send_binary` *(default: false)*: if true, the `bin.path` parameter specifies a file in the Contest filesystem that is going to be copied to the target machine thru the ssh channel (in /tmp) before starting execution. The default false means that the `bin.path` option specifies an existing remote file on the target machine and no transfers take place.
- `async` *(default: omit)*: see below

For long running remote jobs (for now, just ssh based), the async option specifies the remote agent that will be used to monitor the job. For convenience, an implementation that satisfies the agent protocol for this transport is provided in `cmds/exec_agent`.
To use it `go build` the exec_agent binary and specify the binary path in the `agent` key. This binary must be present on the same filesystem Contest is running on.
The `time_quota` *(default: 0)* option inside `async` gets passed onto the agent when it launches a job. If this is exceeded, the remote agent just kills the controlled process and exits. This is useful in the case of network partitions or Contest server errors because it enforces that resources are eventually released on the target machines. The default means infinite quota, so it is recommended to set it to some appropriate value.

The top level `time_quota` applies to individual ssh operations. So when `async` is omitted, the live ssh connection (and process running remotely) gets killed when the quota is exceeded. For async processes, the individual start, poll, etc operations are monitored for execution time, making this option less relevant in this case.

## Examples

The following launches `/home/test/exec_bin` on the same machine as the Contest server and runs it for at most 20 seconds per target with the only argument being the target FQDN. The result is determined from the exit code.
```
"Steps": [
    {
        "name": "exec",
        "label": "label",
        "parameters": {
            "bag": [
                {
                    "bin": {
                        "path": "/home/test/exec_bin",
                        "args": [
                            "{{.FQDN}}"
                        ]
                    },
                    "transport": {
                        "proto": "local"
                    },
                    "constraints": {
                        "time_quota": "20s"
                    }
                }
            ]
        }
    }
]
```

The following launches the file `/packages/exec_bin` residing on the test target thru ssh, which was authenticated using pubkey method. The process and connection are killed after 20s (and test fails) if that quota is exceeded. The binary produces OCP T&V output, which is parsed by Contest to decide the pass/fail result.
```
"Steps": [
    {
        "name": "exec",
        "label": "label",
        "parameters": {
            "bag": [
                {
                    "bin": {
                        "path": "/packages/exec_bin"
                    },
                    "transport": {
                        "proto": "ssh",
                        "options": {
                            "host": "{{.FQDN}}",
                            "user": "test",
                            "identity_file": "/home/test/.ssh/id_rsa",
                        }
                    },
                    "constraints": {
                        "time_quota": "20s"
                    },
                    "ocp_output": true
                }
            ]
        }
    }
]
```

The following config copies the `/home/test/exec_bin `binary to the target machine (in `/tmp/<random_uuid_name>`), along with a copy of the agent taken from `/home/test/contest/cmds/exec_agent/exec_agent`. The agent is then started (which starts the remote binary itself) then the ssh connection is terminated. Contest then periodically establishes new ssh connections to poll the outputs and state of the remote agent-controlled process. When the agent goes over the 20s quota, it kills the process (regardless of anything Contest might be doing). The test result is parsed from the OCP T&V output.
```
"Steps": [
    {
        "name": "exec",
        "label": "label",
        "parameters": {
            "bag": [
                {
                    "bin": {
                        "path": "/home/test/exec_bin"
                    },
                    "transport": {
                        "proto": "ssh",
                        "options": {
                            "host": "{{.FQDN}}",
                            "user": "test",
                            "identity_file": "/home/test/.ssh/id_rsa",
                            "send_binary": true,
                            "async": {
                                "agent": "/home/test/contest/cmds/exec_agent/exec_agent",
                                "time_quota": "20s"
                            }
                        }
                    },
                    "ocp_output": true
                }
            ]
        }
    }
]
```
