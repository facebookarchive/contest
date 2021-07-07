# ConTest’s Step Plugin Author’s Guide

## Why write a step plugin?

At the heart of a Contest job is a sequence of steps - actions, which we want to perform on selected targets. There are  multiple step types, and each type has to be implemented as a step plugin. Some step plugins like “sshcmd” or “echo” are available out of the box, but in order to do something more complex like calling external APIs or parsing complex data structures we need to write our custom step plugins. 

## Step Plugin in a Job Descriptor

Contest job descriptor refers step plugins inside of the `Steps` array.  `name` field corresponds to a plugin’s name, returned by `Name()` method of the plugin. `label` is a label of the step. Multiple steps can use the same plugin. `parameters` object is a set of plugin dependent parameters. 


```
"Steps": [
     {
          "name": "cmd",
          "label": "some label",
          "parameters": {
             "executable": ["sleep"],
             "args": ["40"]
        }
      },
      {
          "name": "fbar",
          "label": "fbar_suppress",
          "parameters": {
              "action":["blockTarget"],
              "blockTargetDuration":["10m"]
          }
      },
      {
          "name": "fbar",
          "label": "fbar_unsuppress",
          "parameters": {
              "action":["unblockTarget"]
          }
      }
]
```

In this example we see three steps, one of which uses `cmd` step plugin and two others - `fbar` step plugin.


## Step Plugin Interface

Step plugin is a Go struct which implements `TestStep`  interface. The interface is defined in [pkg/test/step.go](https://github.com/facebookincubator/contest/blob/master/pkg/test/step.go) and it looks like this:

```
// TestStep is the interface that all steps need to implement to be executed
// by the TestRunner
type TestStep interface {
    // Name returns the name of the step
    Name() string
    // Run runs the test step. The test step is expected to be synchronous.
    Run(ctx xcontext.Context, ch TestStepChannels, params TestStepParameters, ev testevent.Emitter,
        resumeState json.RawMessage) (json.RawMessage, error)
    // ValidateParameters checks that the parameters are correct before passing
    // them to Run.
    ValidateParameters(ctx xcontext.Context, params TestStepParameters) error
}
```

`Name` method returns a string - name of the plugin we saw in the descriptor. There is no hard limit for the name’s length, but it should be concise and expose plugin’s purpose. It is **case insensitive** and must be **unique** among registered step plugins.
`Run` is the heart of the plugin. It’s a method where the main code lives. It’s discussed in the “Implementing Run()” chapter. `ValidateParameters`, yeah, validates parameters and we will discuss it in the “Working with parameters” chapter. 
There are two additional functions, which also should be implemented: `New` and `Load`. Usually they reside in the same module with the interface implementation. Function `New` returns a reference to a new instance of the plugin (implementation of the TestStep interface) and will be used by Contest to create plugin instances. Function Load is an entry point of the plugin. It’s used during registration process and reports everything Contest needs to know about our plugin. Implementation of the Load function could look like this:

```
// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
    return Name, New, Events
}
```

It takes no arguments and returns three values:

* Name - is a string. Must match the string which Name() method returns
* New - is a factory function we discussed above
* Events - list of event names, will be discussed in the “Emitting events” chapter. It’s an empty list if your plugin does not emit any events.

## Plugin Instance Life Cycle

Contest does not support dynamic plugin loading. All step plugins must be present at compile time and explicitly registered before serving requests. (see “Adding a plugin to your build”). 
The first step in plugin’s life is a call to its `Load` function. It’s called once during startup and is not supposed to create a plugin instance. Contest creates instances of step plugins during job descriptor parsing. One instance of a plugin will be created for each step using it. So if there are three steps referring our plugin - three instances of the plugin are created. Jobs with the same descriptor doe not reuse plugins instances. 
Immediately after plugin instance creation, Contest calls `ValidateParameters` method of the instance and passes `parameters` object, which extracted from the step description. So `ValidateParameters` will be called once for each plugin instance. Plugin instance’s `Run` method will be called once per test run, but only sequentially, so it does not have to be thread safe.
There is no special cleanup call for plugin instance, so if the plugin requires some resources like a socket, one can acquire it in the beginning of the `Run` method and release in `defer`.
As a part of job model recreation Contest instantiates step plugins on each job status request, but their `Run` method will never be called.

Working with parameters
Plugin specific parameters can be passed to a plugin instance through `parameters` member of the step descriptor. 

## Pause/resume and Cancellation Support

Sometimes Contest may need to pause a job. The most common reason for this is restart of the contest process itself. As a plugin author you may need to assist in orderly shutdown if possible. There are two things that can happen to your test step plugin:

* A **cancellation** may come in (`ctx.Done()`). This means the job is cancelled or failed entirely and nothing can be done to save it. Plugins SHOULD immediately cease all actions and return. Plugins MUST return within roughly 30s. Plugins do not have to deal with targets/DUTs, you don’t need to put them into any channels or similar - the job is dead. Return `xcontext.ErrCancelled`
* A **pause request** may come in. This means the contest server wants to restart. Not all plugins can support this easily, you can choose to ignore the pause request and continue as normal, however we want to minimize or completely get rid of these cases in the future. Plugin can either ignore the signal or react to it by serializing the state and returning it as `json.RawMessage`  together with `xcontext.ErrPaused`. If you do this, the framework will call Run again after the server has restarted and give your json back in `resumeState`. Note targets will not be re-injected, you need to remember them. However, new targets can still arrive after the resumption. You don’t need to remember or care about targets that you already returned with pass/fail, the framework knows and will send them to the following step if required.

Further details on the semantics of Pause.

* Handling pause is optional for plugins. So, to pause or not to pause?
    * If your plugin finishes quickly (specified by `--pauseTimeout`, let’s say ~30s), you can just continue.
    * If the plugin cannot pause - e.g. executing an arbitrary command that may have unknown side effects, it also should just continue and hope that target finishes before the timeout.
    * An example of a good case for implementing pause: a plugin that launches an external job and wait for it to complete, polling for its status. In this case, upon receiving pause, it should serialize all the targets in flight at the moment along with job tokens, return them, and continue waiting in the next instance with the same targets and tokens.
* When pausing, ConTest will close the plugin’s input channel signaling that no more targets will be coming during lifetime of this Step object. This is done in addition to asserting the pause signal so the simplest plugins do not need to handle it separately. (Note: more targets can come in after resumption)
* Paused plugin retains responsibility for the targets in flight at the time of pausing, i.e. ones that were injected but for which no result was received yet.
* Successful pause is communicated by returning `ErrPaused` from the Run method. If the plugin was responsible for any targets at the time, any other return value (including `nil`) will be considered a failure to pause and will abort the job.
* Cancellation will follow pause if timeout is about to expire, so both conditions may be asserted on a context, keep that in mind when testing for them.
* When there are no targets in flight and pause is requested, returning `ErrPaused` is allowed (for simplicity). It may be accompanied by `nil` . In general, return value of a plugin not responsible for any targets does not matter.
* When successfully paused, a new instance of the plugin may be resumed with the state returned but it is not a guarantee: another step may have failed to pause correctly in which case entire job would have been aborted and steps that did pause correctly will not be revived.
* On resumption, targets the plugin was responsible for will not be reinjected but plugin is still expected to produce results for them. Therefore, state returned by the plugin must include everything necessary for the new instance to produce results for targets that had been injected into the previous instance.
* Cancel signal may be asserted together with pause, in which case cancellation takes precedence. If both pause and cancel are asserted, there is no need to perform pause-related activities anymore.
* Pause request may be escalated to cancel after some time (e.g. as exit time of the process approaches).


There is a helper function for you that takes care of 90% of all the pause handling for you: [ForEachTargetWithResume](https://github.com/facebookincubator/contest/blob/6867745bd02d2e8f0686173f838c33e2513447bb/plugins/teststeps/teststeps.go#L124). It just takes a simple per-target function you write and handles most of the channel juggling and remembering the targets for you. All you need to do is save state per target (if there is anything to save). 

## Examples

* [example](https://github.com/facebookincubator/contest/blob/master/plugins/teststeps/example/example.go) is a simple plugin without pause/resume support.
* [sleep](https://github.com/facebookincubator/contest/blob/master/plugins/teststeps/sleep/sleep.go) is a good example of a plugin that supports pause/resume.
