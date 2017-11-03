#!/bin/bash
set -e

echo Running Unit Tests
go test ./...

go get -u github.com/robertkrimen/godocdown/godocdown

godocdown > README.md

