#!/bin/bash -e

rm $(which godep)
go install github.com/tools/godep
