package main

import (
  tcclient "github.com/taskcluster/taskcluster-client-go"
  queue "github.com/taskcluster/taskcluster-client-go/queue"
)

type Artifact struct {
  queue *queue.Queue
  agent client
}

func New(creds *tcclient.Credentials) *Artifact {
  q := queue.New(creds)
  a := newAgent()
  return &Artifact{q, a}
}

func (a *Artifact) Upload(taskId, runId, name string) error {
	return nil
}

func (a *Artifact) Download(taskId, runId, name, output string) error {
	return nil
}
