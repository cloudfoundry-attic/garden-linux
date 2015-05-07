package main

/*
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <errno.h>
#include <fcntl.h>
#include <sys/param.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>
#include <linux/sched.h>

int setns(int fd, int nstype);

int enterns() {
  int rv;
  int mntnsfd;

  char mntnspath[PATH_MAX];
  rv = snprintf(mntnspath, sizeof(mntnspath), "/proc/%s/ns/mnt", getenv("TARGET_NS_PID"));
  if(rv == -1) {
    perror("snprintf ns mnt path");
    return 1;
  }

	printf("%s", mntnspath);

  mntnsfd = open(mntnspath, O_RDONLY);
  if(mntnsfd == -1) {
    perror("open mnt namespace");
    return 1;
  }

  rv = setns(mntnsfd, CLONE_NEWNS);
  if(rv == -1) {
    perror("setns");
    return 1;
  }
  close(mntnsfd);

  return 0;
}

__attribute__((constructor)) void init(void) {
	enterns();
}
*/
import "C"

import (
	"flag"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
)

func main() {
	rootfs := flag.String("rootfs", "", "path to pivot into")
	flag.Parse()

	rootfsEnterer := &system.RootFS{*rootfs}
	if err := rootfsEnterer.Enter(); err != nil {
		panic(err)
	}
}
