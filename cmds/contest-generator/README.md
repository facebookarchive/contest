# ConTest Generator

`contest-generator` is a tool used to build the ConTest server with the plugins
you want.

We provide a standard [main.go](/cmds/contest/main.go), where we only include
the most common plugins, but you can use `contest-generator` to customize your
own, with your own set of plugins.

You need two steps:
* define your plugins in a configuration file
* run `contest-generator` with that configuration file


 See [core-plugins.yml](/cmds/contest-generator/core-plugins.yml) for an example
 of a configuration file, or keep reading for a description of the configuration
 format.

## Configuration format

The configuration file has one section per plugin type, and each section has a
list of import paths. For example:

```
targetmanagers:
  - github.com/facebookincubator/contest/plugins/targetmanagers/targetlist
```

In this example, `targetmanagers` is the plugin type, and it will be registered
as a TargetManager object, and `github.com/.../targetlist` is the actual import
path.

There are four plugin types:
* `targetmanagers`: at least one is needed
* `testfetchers`: at least one is needed
* `teststeps`: at least one is needed
* `reporters`: at least one is needed

Duplicate package names are not allowed, because they will result in ambiguous
imports in the output code.
If you have multiple import paths with the same package name, you can specify
an optional `import_alias` field.

The following syntax will generate an error:

```
targetmanagers:
  - path: github.com/facebookincubator/contest/plugins/targetmanagers/blah

teststeps:
  - path: github.com/facebookincubator/contest/plugins/testfetchers/blah
```

because there is no way to distinguish which package `blah` refers to. So you
can use:

```
targetmanagers:
  - path: github.com/facebookincubator/contest/plugins/targetmanagers/blah
    alias: tm_blah

teststeps:
  - path: github.com/facebookincubator/contest/plugins/teststeps/blah
    alias: ts_blah
```


```
$ go run .
2021/08/17 11:14:18 Generating output file '/tmp/contest846469752/contest.go' with the following plugins):
TargetManagers
    github.com/facebookincubator/contest/plugins/targetmanagers/targetlist
    github.com/facebookincubator/contest/plugins/targetmanagers/csvtargetmanager
TestFetchers
    github.com/facebookincubator/contest/plugins/testfetchers/literal
    github.com/facebookincubator/contest/plugins/testfetchers/uri
TestSteps
    github.com/facebookincubator/contest/plugins/teststeps/cmd => ts_cmd
    github.com/facebookincubator/contest/plugins/teststeps/sshcmd
    github.com/facebookincubator/contest/plugins/teststeps/sleep
Reporters
    github.com/facebookincubator/contest/plugins/reporters/targetsuccess
2021/08/17 11:14:18 Generated file '/tmp/contest846469752/contest.go'. You can build it by running 'go build' in the output directory.
/tmp/contest846469752
```

You can also specify an alternative file with the list of plugins:
```
$ ./contest-generator --from my-core-plugins.yml
```

See `./contest-generator -h` for the full list of command line arguments.

Notice that it creates a file called `contest.go` in a temporary directory. To build it, go to the output directory (in the above example `/tmp/contest822776154/`), then run
```
go mod init contest # skip this if you are not using Go modules
go mod tidy         # skip this if you are not using Go modules
go build
```

and it will create an executable called `contest`, which is your custom ConTest server.
