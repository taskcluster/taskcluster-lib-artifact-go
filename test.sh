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
go vet ./...

echo Running govet

go get -u github.com/robertkrimen/godocdown/godocdown

godoc2ghmd $(git config --get remote.origin.url | sed "s,^https://,, ; s,:,/,") > README.md

