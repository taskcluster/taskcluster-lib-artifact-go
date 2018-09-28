#!/bin/bash
set -e

echo Updating dependencies
go get -u ./...
go get -u github.com/golang/lint/golint
go get -u github.com/alecthomas/gometalinter
go get -u github.com/GandalfUK/godoc2ghmd
gometalinter --install > /dev/null

echo Running Unit Tests
go test ./...

echo Linting
golint --min_confidence=0.3 ./...

echo Running govet
go vet ./...


go get -u github.com/robertkrimen/godocdown/godocdown

cat << EOF > README.md
[![Taskcluster CI Status](https://github.taskcluster.net/v1/repository/taskcluster/taskcluster-lib-artifact-go/master/badge.svg)](https://github.taskcluster.net/v1/repository/taskcluster/taskcluster-lib-artifact-go/master/latest)
[![GoDoc](https://godoc.org/github.com/taskcluster/taskcluster-lib-artifact-go?status.svg)](https://godoc.org/github.com/taskcluster/taskcluster-lib-artifact-go)
[![License](https://img.shields.io/badge/license-MPL%202.0-orange.svg)](http://mozilla.org/MPL/2.0)

EOF
godoc2ghmd $(git config --get remote.origin.url | sed "s,^https://,, ; s,:,/,") >> README.md

