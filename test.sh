#!/bin/bash
set -e


echo Updating dependencies
go get -u ./...
go get -u github.com/robertkrimen/godocdown/godocdown
go get -u github.com/golang/lint/golint
go get -u github.com/alecthomas/gometalinter

echo Running Unit Tests
go test ./...

echo Linting
golint --min_confidence=0.3 ./...
go vet ./...

echo Running govet

go get -u github.com/robertkrimen/godocdown/godocdown

godocdown > README.md

