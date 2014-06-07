# this builds the base image for Warden Linux's CI
#
# based on mischief/docker-golang, updated for ubuntu trusty

FROM ubuntu:14.04

ENV HOME /root
ENV GOPATH /root/go
ENV PATH /root/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games

RUN mkdir -p /root/go

RUN echo "deb http://mirror.anl.gov/pub/ubuntu trusty main universe" > /etc/apt/sources.list
RUN apt-get update
RUN apt-get install -y build-essential mercurial git-core subversion wget

RUN wget -qO- https://storage.googleapis.com/golang/go1.2.2.linux-amd64.tar.gz | tar -C /usr/local -xzf -

# pull in dependencies for the server
RUN apt-get -y install iptables quota rsync net-tools protobuf-compiler

# pull in the prebuilt rootfs
ADD warden-test-rootfs.tar /opt/warden/rootfs

# install the binary for generating the protocol
RUN go get code.google.com/p/gogoprotobuf/protoc-gen-gogo
