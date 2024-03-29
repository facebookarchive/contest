// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"syscall"
)

type PollResponse struct {
	Stdout []byte
	Stderr []byte
	Alive  bool
}

type monitor struct {
	proc *os.Process

	// TODO: these should really be readonly
	stdout *SafeBuffer
	stderr *SafeBuffer

	reaper *SafeSignal

	// there really shouldnt be multiple concurrent consumers
	// by design, but better be safe than sorry
	mu sync.Mutex
}

func newMonitor(proc *os.Process, stdout *SafeBuffer, stderr *SafeBuffer, reaper *SafeSignal) *monitor {
	return &monitor{proc, stdout, stderr, reaper, sync.Mutex{}}
}

func (m *monitor) Poll(_ int, reply *PollResponse) error {
	log.Printf("got a call for: poll")
	m.mu.Lock()
	defer m.mu.Unlock()

	reply.Stdout = make([]byte, m.stdout.Len())
	if _, err := m.stdout.Read(reply.Stdout); err != nil {
		return fmt.Errorf("failed to read stdout: %w", err)
	}

	reply.Stderr = make([]byte, m.stderr.Len())
	if _, err := m.stderr.Read(reply.Stderr); err != nil {
		return fmt.Errorf("failed to read stderr: %w", err)
	}

	// a signal value of 0 can be sent to the process to probe whether it's still alive
	// or not, it triggers no handling in the receiver process; apart from this, there
	// aren't many other ways of checking the process health (apart from poking the pid directly)
	reply.Alive = true
	if err := m.proc.Signal(syscall.Signal(0)); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			reply.Alive = false
			return nil
		}

		return fmt.Errorf("failed to send signal: %w", err)
	}

	return nil
}

func (m *monitor) Kill(_ int, _ *interface{}) error {
	log.Print("got a call for: kill")

	if err := m.proc.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("failed to send SIGKILL: %w", err)
	}

	return nil
}

func (m *monitor) Wait(_ int, _ *interface{}) error {
	log.Print("got a call for: wait")

	m.reaper.Signal()
	return nil
}

const sockFormat = "/tmp/exec_bin_sock_%d"

type MonitorServer struct {
	addr string
	mon  *monitor

	http *http.Server
}

func NewMonitorServer(proc *os.Process, stdout *SafeBuffer, stderr *SafeBuffer, reap *SafeSignal) *MonitorServer {
	addr := fmt.Sprintf(sockFormat, proc.Pid)
	mon := newMonitor(proc, stdout, stderr, reap)

	return &MonitorServer{addr, mon, nil}
}

func (m *MonitorServer) Serve() error {
	log.Printf("starting monitor...")

	if err := os.RemoveAll(m.addr); err != nil {
		return fmt.Errorf("failed to clear lingering socket %s: %w", m.addr, err)
	}

	listener, err := net.Listen("unix", m.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", m.addr, err)
	}
	defer listener.Close()

	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("api", m.mon); err != nil {
		return fmt.Errorf("failed to register rpc api: %v", err)
	}

	log.Printf("starting RPC server at: %s", m.addr)
	m.http = &http.Server{
		Addr:    m.addr,
		Handler: rpcServer,
	}
	return m.http.Serve(listener)
}

func (m *MonitorServer) Shutdown() error {
	log.Printf("shutting down monitor...")

	if err := os.RemoveAll(m.addr); err != nil {
		return fmt.Errorf("failed to remove any socket %s: %w", m.addr, err)
	}

	if m.http != nil {
		// dont care about cancellation context
		return m.http.Shutdown(context.Background())
	}
	return nil
}

type ErrCantConnect struct {
	w error
}

func (e ErrCantConnect) Error() string {
	return e.w.Error()
}

func (e ErrCantConnect) Unwrap() error {
	return e.w
}

type MonitorClient struct {
	addr string
}

func NewMonitorClient(pid int) *MonitorClient {
	addr := fmt.Sprintf(sockFormat, pid)
	return &MonitorClient{addr}
}

func (m *MonitorClient) Wait() error {
	client, err := rpc.DialHTTP("unix", m.addr)
	if err != nil {
		return &ErrCantConnect{fmt.Errorf("failed to connect to %s: %w", m.addr, err)}
	}
	defer client.Close()

	var reply interface{}
	if err := client.Call("api.Wait", 0, &reply); err != nil {
		return fmt.Errorf("failed to call rpc method: %w", err)
	}

	return nil
}

func (m *MonitorClient) Poll() (*PollResponse, error) {
	client, err := rpc.DialHTTP("unix", m.addr)
	if err != nil {
		return nil, &ErrCantConnect{fmt.Errorf("failed to connect to %s: %w", m.addr, err)}
	}
	defer client.Close()

	var reply PollResponse
	if err := client.Call("api.Poll", 0, &reply); err != nil {
		return nil, fmt.Errorf("failed to call rpc method: %w", err)
	}

	return &reply, nil
}

func (m *MonitorClient) Kill() error {
	client, err := rpc.DialHTTP("unix", m.addr)
	if err != nil {
		return &ErrCantConnect{fmt.Errorf("failed to connect to %s: %w", m.addr, err)}
	}
	defer client.Close()

	var reply interface{}
	if err := client.Call("api.Kill", 0, &reply); err != nil {
		return fmt.Errorf("failed to call rpc method: %w", err)
	}

	return nil
}
