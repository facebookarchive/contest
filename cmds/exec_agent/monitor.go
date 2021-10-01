// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"syscall"
)

type BufferData struct {
	Data []byte
}

type monitorAPI struct {
	// TODO: these should really be readonly
	stdout *bytes.Buffer
	stderr *bytes.Buffer

	done chan<- struct{}
}

func newMonitorAPI(stdout *bytes.Buffer, stderr *bytes.Buffer, done chan<- struct{}) *monitorAPI {
	return &monitorAPI{stdout, stderr, done}
}

func (api *monitorAPI) Output(fd int, reply *BufferData) error {
	log.Printf("got a call for: output/%d", fd)

	switch fd {
	case syscall.Stdout:
		reply.Data = make([]byte, api.stdout.Len())

	case syscall.Stderr:
		reply.Data = make([]byte, api.stderr.Len())

	default:
		return fmt.Errorf("unknown file descriptor: %d", fd)
	}

	_, err := api.stdout.Read(reply.Data)
	return err
}

func (api *monitorAPI) Wait(_ int, _ *interface{}) error {
	log.Print("got a call for: wait")

	close(api.done)
	return nil
}

const sockFormat = "/tmp/exec_bin_sock_%d"

type MonitorServer struct {
	addr string
	api  *monitorAPI

	http *http.Server
}

func NewMonitorServer(pid int, stdout *bytes.Buffer, stderr *bytes.Buffer, done chan<- struct{}) *MonitorServer {
	addr := fmt.Sprintf(sockFormat, pid)
	api := newMonitorAPI(stdout, stderr, done)

	return &MonitorServer{addr, api, nil}
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
	rpcServer.RegisterName("api", m.api)

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

type OutputFD int

const (
	Stdout = OutputFD(1)
	Stderr = OutputFD(2)
)

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
		return fmt.Errorf("failed to connect to %s: %w", m.addr, err)
	}
	defer client.Close()

	var reply interface{}
	if err := client.Call("api.Wait", 0, &reply); err != nil {
		return fmt.Errorf("failed to call rpc method: %w", err)
	}
	return nil
}

func (m *MonitorClient) Output(fd OutputFD) ([]byte, error) {
	client, err := rpc.DialHTTP("unix", m.addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", m.addr, err)
	}
	defer client.Close()

	var reply BufferData
	if err := client.Call("api.Output", int(fd), &reply); err != nil {
		return nil, fmt.Errorf("failed to call rpc method: %w", err)
	}

	return reply.Data, nil
}
