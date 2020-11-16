// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package sshcmd

// The SSHCmd plugin implements an SSH command executor step. Only PublicKey
// and Password authentication are supported. GSSAPI not supported yet.
//
// Warning: this plugin does not lock password and keys in memory, and does no
// safe erase in memory to avoid forensic attacks. If you need that, please
// submit a PR.
//
// Warning: commands are interpreted, so be careful with external input in the
// test step arguments.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/cerrors"
	"github.com/facebookincubator/contest/pkg/event"
	"github.com/facebookincubator/contest/pkg/event/testevent"
	"github.com/facebookincubator/contest/pkg/logging"
	"github.com/facebookincubator/contest/pkg/target"
	"github.com/facebookincubator/contest/pkg/test"
	"github.com/facebookincubator/contest/plugins/teststeps"
	shellquote "github.com/kballard/go-shellquote"
	"golang.org/x/crypto/ssh"
)

// Name is the name used to look this plugin up.
var Name = "SSHCmd"

var log = logging.GetLogger("teststeps/" + strings.ToLower(Name))

// Events is used by the framework to determine which events this plugin will
// emit. Any emitted event that is not registered here will cause the plugin to
// fail.
var Events = []event.Name{}

const defaultSSHPort = 22
const defaultTimeoutParameter = "10m"

// SSHCmd is used to run arbitrary commands as test steps.
type SSHCmd struct {
	Host           *test.Param
	Port           *test.Param
	User           *test.Param
	PrivateKeyFile *test.Param
	Password       *test.Param
	Executable     *test.Param
	Args           []test.Param
	Expect         *test.Param
	Timeout        *test.Param
}

// Name returns the plugin name.
func (ts SSHCmd) Name() string {
	return Name
}

// Run executes the cmd step.
func (ts *SSHCmd) Run(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.Emitter) error {
	// XXX: Dragons ahead! The target (%t) substitution, and function
	// expression evaluations are done at run-time, so they may still fail
	// despite passing at early validation time.
	// If the function evaluations called in validateAndPopulate are not idempotent,
	// the output of the function expressions may be different (e.g. with a call to a
	// backend or a random pool of results)
	// Function evaluation could be done at validation time, but target
	// substitution cannot, because the targets are not known at that time.
	if err := ts.validateAndPopulate(params); err != nil {
		return err
	}

	f := func(cancel, pause <-chan struct{}, target *target.Target) error {
		// apply filters and substitutions to user, host, private key, and command args
		user, err := ts.User.Expand(target)
		if err != nil {
			return fmt.Errorf("cannot expand user parameter: %v", err)
		}

		host, err := ts.Host.Expand(target)
		if err != nil {
			return fmt.Errorf("cannot expand host parameter: %v", err)
		}

		portStr, err := ts.Port.Expand(target)
		if err != nil {
			return fmt.Errorf("cannot expand port parameter: %v", err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("failed to convert port parameter to integer: %v", err)
		}

		timeoutStr, err := ts.Timeout.Expand(target)
		if err != nil {
			return fmt.Errorf("cannot expand timeout parameter %s: %v", timeoutStr, err)
		}

		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return fmt.Errorf("cannot parse timeout paramter: %v", err)
		}

		timeTimeout := time.Now().Add(timeout)

		// apply functions to the private key, if any
		var signer ssh.Signer
		privKeyFile, err := ts.PrivateKeyFile.Expand(target)
		if err != nil {
			return fmt.Errorf("cannot expand private key file parameter: %v", err)
		}

		if privKeyFile != "" {
			key, err := ioutil.ReadFile(privKeyFile)
			if err != nil {
				return fmt.Errorf("cannot read private key at %s: %v", ts.PrivateKeyFile, err)
			}
			signer, err = ssh.ParsePrivateKey(key)
			if err != nil {
				return fmt.Errorf("cannot parse private key: %v", err)
			}
		}

		password, err := ts.Password.Expand(target)
		if err != nil {
			return fmt.Errorf("cannot expand password parameter: %v", err)
		}

		auth := []ssh.AuthMethod{}
		if signer != nil {
			auth = append(auth, ssh.PublicKeys(signer))
		}
		if password != "" {
			auth = append(auth, ssh.Password(password))
		}

		config := ssh.ClientConfig{
			User: user,
			Auth: auth,
			// TODO expose this in the plugin arguments
			//HostKeyCallback: ssh.FixedHostKey(hostKey),
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		executable, err := ts.Executable.Expand(target)
		if err != nil {
			return fmt.Errorf("cannot expand executable parameter: %v", err)
		}

		// apply functions to the command args, if any
		var args []string
		for _, arg := range ts.Args {
			earg, err := arg.Expand(target)
			if err != nil {
				return fmt.Errorf("cannot expand command argument '%s': %v", arg, err)
			}
			args = append(args, earg)
		}

		// connect to the host
		addr := net.JoinHostPort(host, strconv.Itoa(int(port)))
		client, err := ssh.Dial("tcp", addr, &config)
		if err != nil {
			return fmt.Errorf("cannot connect to SSH server %s: %v", addr, err)
		}
		defer func() {
			if err := client.Close(); err != nil {
				log.Warningf("Failed to close SSH connection to %s: %v", addr, err)
			}
		}()
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("cannot create SSH session to server %s: %v", addr, err)
		}
		defer func() {
			if err := session.Close(); err != nil && err != io.EOF {
				log.Warningf("Failed to close SSH session to %s: %v", addr, err)
			}
		}()
		// run the remote command and catch stdout/stderr
		var stdout, stderr bytes.Buffer
		session.Stdout, session.Stderr = &stdout, &stderr
		cmd := shellquote.Join(append([]string{executable}, args...)...)
		log.Printf("Running remote SSH command on %s: '%v'", addr, cmd)
		errCh := make(chan error)
		go func() {
			innerErr := session.Run(cmd)
			errCh <- innerErr
		}()

		expect := ts.Expect.String()
		for {
			select {
			case err := <-errCh:
				log.Infof("Stdout of command '%s' is '%s'", cmd, stdout.Bytes())
				if err == nil {
					// Execute expectations
					if expect == "" {
						log.Warningf("no expectations specified")
					} else {
						re := regexp.MustCompile(expect)
						matches := re.FindAll(stdout.Bytes(), -1)
						if len(matches) > 0 {
							log.Infof("match for regex \"%s\" found", expect)
						} else {
							return fmt.Errorf("match for %s not found for target %v", expect, target)
						}
					}
				} else {
					log.Warningf("Stderr of command '%s' is '%s'", cmd, stderr.Bytes())
				}
				return err
			case <-cancel:
				return session.Signal(ssh.SIGKILL)
			case <-pause:
				return session.Signal(ssh.SIGKILL)
			default:
				re := regexp.MustCompile(expect)
				matches := re.FindAll(stdout.Bytes(), -1)
				if len(matches) > 0 {
					log.Infof("match for regex \"%s\" found", expect)
					return nil
				}
				if time.Now().After(timeTimeout) {
					return fmt.Errorf("timed out after %s", timeout)
				}
				session.Signal(ssh.Signal("CONT"))
				// Sanity Break to not spam the Server
				time.Sleep(250 * time.Millisecond)
			}
		}
	}
	return teststeps.ForEachTarget(Name, cancel, pause, ch, f)
}

func (ts *SSHCmd) validateAndPopulate(params test.TestStepParameters) error {
	var err error
	ts.Host = params.GetOne("host")
	if ts.Host.IsEmpty() {
		return errors.New("invalid or missing 'host' parameter, must be exactly one string")
	}
	if params.GetOne("port").IsEmpty() {
		ts.Port = test.NewParam(strconv.Itoa(defaultSSHPort))
	} else {
		var port int64
		port, err = params.GetInt("port")
		if err != nil {
			return fmt.Errorf("invalid 'port' parameter, not an integer: %v", err)
		}
		if port < 0 || port > 0xffff {
			return fmt.Errorf("invalid 'port' parameter: not in range 0-65535")
		}
	}

	ts.User = params.GetOne("user")
	if ts.User.IsEmpty() {
		return errors.New("invalid or missing 'user' parameter, must be exactly one string")
	}

	ts.PrivateKeyFile = params.GetOne("private_key_file")
	// do not fail if key file is empty, in such case it won't be used
	ts.PrivateKeyFile = params.GetOne("private_key_file")

	// do not fail if password is empty, in such case it won't be used
	ts.Password = params.GetOne("password")

	ts.Executable = params.GetOne("executable")
	if ts.Executable.IsEmpty() {
		return errors.New("invalid or missing 'executable' parameter, must be exactly one string")
	}
	ts.Args = params.Get("args")
	ts.Expect = params.GetOne("expect")

	if params.GetOne("timeout").IsEmpty() {
		ts.Timeout = test.NewParam(defaultTimeoutParameter)
	} else {
		ts.Timeout = params.GetOne("timeout")
	}
	return nil
}

// ValidateParameters validates the parameters associated to the TestStep
func (ts *SSHCmd) ValidateParameters(params test.TestStepParameters) error {
	log.Printf("Params %+v", params)
	return ts.validateAndPopulate(params)
}

// Resume tries to resume a previously interrupted test step. SSHCmd cannot
// resume.
func (ts *SSHCmd) Resume(cancel, pause <-chan struct{}, ch test.TestStepChannels, params test.TestStepParameters, ev testevent.EmitterFetcher) error {
	return &cerrors.ErrResumeNotSupported{StepName: Name}
}

// CanResume tells whether this step is able to resume.
func (ts *SSHCmd) CanResume() bool {
	return false
}

// New initializes and returns a new SSHCmd test step.
func New() test.TestStep {
	return &SSHCmd{}
}

// Load returns the name, factory and events which are needed to register the step.
func Load() (string, test.TestStepFactory, []event.Name) {
	return Name, New, Events
}
