package controllers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"net/http"
	"strings"
)

type Interface interface {
	GetConfig() *rest.Config
	GetClient() kubernetes.Interface
}

// PodExecutor is the pod executor interface
type PodExecutor interface {
	Execute(ctx context.Context, namespace, name, containerName, command string) (io.Reader, error)
}

type podExecutor struct {
	client Interface
}


type executorClient struct{
	config *rest.Config
	client kubernetes.Interface
}

func (e *executorClient) GetConfig() *rest.Config{
	return e.config
}

func (e *executorClient) GetClient() kubernetes.Interface{
	return e.client
}

// Execute executes a command on a pod
func (p *podExecutor) Execute(ctx context.Context, namespace, name, containerName, command string) (io.Reader, error) {
	var stdout, stderr bytes.Buffer
	request := p.client.GetClient().CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(name).
		Namespace(namespace).
		SubResource("exec").
		Param("container", containerName).
		Param("command", "/bin/sh").
		Param("stdin", "true").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "false").
		Context(ctx)

	executor, err := remotecommand.NewSPDYExecutor(p.client.GetConfig(), http.MethodPost, request.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to initialized the command exector: %v", err)
	}

	err = executor.Stream(remotecommand.StreamOptions{
		Stdin:  strings.NewReader(command),
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return &stderr, err
	}

	return &stdout, nil
}
