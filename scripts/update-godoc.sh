#!/usr/bin/env bash
set -e
set -x

repoPath=$(cd $(dirname $BASH_SOURCE)/.. && pwd)

if [ -z $GOROOT ]; then
  export GOROOT=/usr/local/go
  export PATH=$GOROOT/bin:$PATH
fi

if [ -z $GOPATH ]; then
  export GOPATH=$HOME/go
  export PATH=$GOPATH/bin:$PATH
fi

cd $(dirname $0)/..
go get -u -v github.com/tools/godep
godep restore
ln -s $PWD $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux

cd $GOPATH/src/github.com/cloudfoundry-incubator/garden-linux
go run scripts/update-godoc/main.go
