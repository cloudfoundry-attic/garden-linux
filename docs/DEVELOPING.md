# Development Setup and Workflow

## Setting up Go

Install Go 1.3 or later. For example, install [gvm](https://github.com/moovweb/gvm) and issue:
```
# gvm install go1.3
# gvm use go1.3
```

Extend `$GOPATH` and `$PATH`:
```
# export GOPATH=~/go:$GOPATH
# export PATH=~/go/bin:$PATH
```

## Get garden-linux and its dependencies
```
# go get github.com/cloudfoundry-incubator/garden-linux
# cd ~/go/src/github.com/cloudfoundry-incubator/garden-linux
```

## Install Concourse and Fly

- Install concourse following the instructions at [Concourse.ci](http://concourse.ci)
- Install fly as follows:

```
go get github.com/concourse/fly
```

## Run the Integration Tests

To test under `fly` run

```bash
scripts/garden-fly
```

in the repository root.

`garden-fly` provides the necessary parameters to `fly` which uses `build.yml`
and runs `scripts/concourse-test` on an existing Concourse instance which must
already be running locally in a virtual machine.
