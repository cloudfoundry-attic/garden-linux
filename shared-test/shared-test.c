#define _GNU_SOURCE

#include <assert.h>
#include <errno.h>
#include <fcntl.h>
#include <sched.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/ipc.h>
#include <sys/mount.h>
#include <sys/param.h>
#include <sys/shm.h>
#include <sys/signalfd.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <termios.h>
#include <unistd.h>

int main(int argc, char **argv) {
  int rv;
  int parent_ns, child_ns;

  parent_ns = open("/proc/self/ns/mnt", O_RDONLY);
  assert(rv != -1);

  rv = unshare(CLONE_NEWNS);
  assert(rv == 0);

  rv = chdir("/");
  assert(rv == 0);

  rv = mkdir("shared", 0755);
  assert(rv == 0);

  rv = mount("shared", "shared", NULL, MS_BIND, NULL);
  assert(rv == 0);

  rv = mount("shared", "shared", NULL, MS_SHARED, NULL);
  assert(rv == 0);

  rv = mkdir("duped", 0755);
  assert(rv == 0);

  rv = mount("shared", "duped", NULL, MS_BIND, NULL);
  assert(rv == 0);

  rv = mkdir("shared/sub-mount-x", 0755);
  assert(rv == 0);

  rv = mkdir("shared/sub-mount-y", 0755);
  assert(rv == 0);

  rv = mount("shared/sub-mount-x", "shared/sub-mount-y", NULL, MS_BIND, NULL);
  assert(rv == 0);

  rv = creat("shared/sub-mount-x/some-file", 0644);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("shared/sub-mount-y/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("duped/sub-mount-x/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("duped/sub-mount-y/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = unshare(CLONE_NEWNS);
  assert(rv == 0);

  child_ns = open("/proc/self/ns/mnt", O_RDONLY);
  assert(rv != -1);

  rv = setns(parent_ns, CLONE_NEWNS);
  assert(rv == 0);

  rv = mkdir("shared/sub-mount-x-2", 0755);
  assert(rv == 0);

  rv = mkdir("shared/sub-mount-y-2", 0755);
  assert(rv == 0);

  rv = mount("shared/sub-mount-x-2", "shared/sub-mount-y-2", NULL, MS_BIND, NULL);
  assert(rv == 0);

  rv = creat("shared/sub-mount-x-2/some-file", 0644);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("shared/sub-mount-y-2/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("duped/sub-mount-x-2/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("duped/sub-mount-y-2/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = setns(child_ns, CLONE_NEWNS);
  assert(rv == 0);

  rv = open("shared/sub-mount-x-2/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("shared/sub-mount-y-2/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("duped/sub-mount-x-2/some-file", O_RDONLY);
  assert(rv != -1);

  rv = close(rv);
  assert(rv == 0);

  rv = open("duped/sub-mount-y-2/some-file", O_RDONLY);
  assert(rv != -1);

  return 0;
}

