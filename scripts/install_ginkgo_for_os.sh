#!/bin/bash -e

rm $(which ginkgo)
go install github.com/onsi/ginkgo/ginkgo
