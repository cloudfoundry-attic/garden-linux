# this builds the base image for Warden Linux's CI
#
# based on mischief/docker-golang, updated for ubuntu trusty

FROM ubuntu:14.04

ENV HOME /root
ENV GOPATH /root/go
ENV PATH /root/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games

RUN mkdir -p /root/go

RUN apt-get update && apt-get install -y build-essential mercurial git-core subversion wget

RUN wget -qO- https://storage.googleapis.com/golang/go1.3.1.linux-amd64.tar.gz | tar -C /usr/local -xzf -

# pull in dependencies for the server
RUN apt-get update && apt-get -y install iptables quota rsync net-tools protobuf-compiler

# pull in the prebuilt rootfs
ADD warden-test-rootfs.tar /opt/warden/rootfs

# install the binary for generating the protocol
RUN go get code.google.com/p/gogoprotobuf/protoc-gen-gogo

# install nsenter
RUN mkdir /tmp/nsenter && \
      wget -qO- https://www.kernel.org/pub/linux/utils/util-linux/v2.24/util-linux-2.24.tar.gz \
      | tar -C /tmp/nsenter -zxf - && \
      cd /tmp/nsenter/* && ./configure --without-ncurses && \
      make CFLAGS=-fPIC LDFLAGS=-all-static nsenter && \
      cp ./nsenter /usr/local/bin
