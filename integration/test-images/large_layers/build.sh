#!/bin/bash -e

DIR=$(dirname $0)
CACHE_DIR=${DIR}/cache
GO_TGZ=${CACHE_DIR}/go.tar.gz

mkdir -p $CACHE_DIR

if [ ! -f $GO_TGZ ]
then
    echo "go not found locally - downloading"
    wget https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz -O $GO_TGZ
else
    echo "go found locally - not downloading"
fi

docker build -t cloudfoundry/large_layers $DIR
