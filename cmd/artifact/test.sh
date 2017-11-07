#!/bin/bash

set -xe

go build
./artifact create-task > testenv
. testenv

artifact upload $TASKID $RUNID public/artifact --input artifact
artifact download $TASKID $RUNID public/artifact --output artifact-downloaded
diff artifact artifact-downloaded
