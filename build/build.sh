#!/usr/bin/env bash
set -e

cd ${GOPATH}/src/github.com/AliyunContainerService/alicloud-storage-provisioner/
GIT_SHA=`git rev-parse --short HEAD || echo "HEAD"`

export GOARCH="amd64"
export GOOS="linux"
cd cmd/disk-provisioner

go build -o disk-provisioner
echo "building finished..."

mv disk-provisioner ../../build/
cd ../../build/


