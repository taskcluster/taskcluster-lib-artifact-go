package main

import (
	"os/exec"
	"testing"
)

type logger struct {
	t *testing.T
	n string
}

func (l *logger) Write(p []byte) (n int, err error) {
	l.t.Logf("STDOUT: %s", p)
	return len(p), nil
}

func TestCLI(t *testing.T) {
	cmd := exec.Command("bash", "./test.sh")
	cmd.Stdout = &logger{t, "STDOUT"}
	cmd.Stderr = &logger{t, "STDERR"}
	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}
}
