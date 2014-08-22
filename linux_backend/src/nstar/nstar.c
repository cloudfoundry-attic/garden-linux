/*
 * This executable passes through to the host's tar, extracting into a
 * directory in the container.
 *
 * It does this with a funky dance involving switching to the container's mount
 * namespace, creating the destination and saving off its fd, and then
 * switching back to the host's rootfs (but the container's destination) for
 * the actual untarring.
 */

#include <stdio.h>
#include <errno.h>
#include <fcntl.h>
#include <linux/sched.h>
#include <pwd.h>
#include <string.h>
#include <sys/param.h>
#include <unistd.h>

/* recursively mkdir with directories owned by a given user */
int mkdir_p_as(const char *dir, uid_t uid, gid_t gid) {
  char tmp[PATH_MAX];
  char *p = NULL;
  size_t len;
  int rv;

  /* copy the given dir as it'll be mutated */
  snprintf(tmp, sizeof(tmp), "%s", dir);
  len = strlen(tmp);

  /* strip trailing slash */
  if(tmp[len - 1] == '/')
    tmp[len - 1] = 0;

  for(p = tmp + 1; *p; p++)
    if(*p == '/') {
      /* temporarily null-terminte the string so that mkdir only creates this
       * path segment */
      *p = 0;

      /* mkdir with truncated path segment */
      rv = mkdir(tmp, 0755);
      if(rv == -1 && errno != EEXIST) {
        return rv;
      }

      rv = chown(tmp, uid, gid);
      if(rv == -1) {
        return rv;
      }

      /* restore path separator */
      *p = '/';
    }

  /* create final destination */
  rv = mkdir(tmp, S_IRWXU);
  if(rv == -1 && errno != EEXIST) {
    return rv;
  }

  return chown(tmp, uid, uid);
}

int main(int argc, char **argv) {
  int rv;
  int nsfd;
  int uid;
  char *destination;
  int tpid;
  int hostrootfd;
  int containerdestfd;
  struct passwd *pw;

  if(argc < 4) {
    fprintf(stderr, "Usage: %s <wshd pid> <uid> <destination>\n", argv[0]);
    return 1;
  }

  rv = sscanf(argv[1], "%d", &tpid);
  if(rv != 1) {
    fprintf(stderr, "invalid pid\n");
    return 1;
  }

  rv = sscanf(argv[2], "%d", &uid);
  if(rv != 1) {
    fprintf(stderr, "invalid uid\n");
    return 1;
  }

  destination = argv[3];

  char nspath[PATH_MAX];
  snprintf(nspath, sizeof(nspath), "/proc/%u/ns/mnt", tpid);

  nsfd = open(nspath, O_RDONLY);
  if(nsfd == -1) {
    perror("open");
    return 1;
  }

  hostrootfd = open("/", O_RDONLY);
  if(hostrootfd == -1) {
    perror("open");
    return 1;
  }

  /* switch to container's mount namespace/rootfs */
  rv = setns(nsfd, CLONE_NEWNS);
  if(rv == -1) {
    perror("setns");
    return 1;
  }

  close(nsfd);

  pw = getpwuid(uid);
  if(pw == NULL) {
    perror("getpwuid");
    return 1;
  }

  rv = chdir(pw->pw_dir);
  if(rv == -1) {
    perror("chdir");
    return 1;
  }

  /* create destination directory */
  rv = mkdir_p_as(destination, uid, uid);
  if(rv == -1 && errno != EEXIST) {
    perror("mkdir");
    return 1;
  }

  /* save off destination dir for switching back to it later */
  containerdestfd = open(destination, O_RDONLY);
  if(containerdestfd == -1) {
    perror("open");
    return 1;
  }

  /* switch to original host rootfs */
  rv = fchdir(hostrootfd);
  if(rv == -1) {
    perror("fchdir");
    return 1;
  }

  rv = chroot(".");
  if(rv == -1) {
    perror("chroot");
    return 1;
  }

  close(hostrootfd);

  /* switch to container's destination directory, with host still as rootfs */
  rv = fchdir(containerdestfd);
  if(rv == -1) {
    perror("fchdir");
    return 1;
  }

  close(containerdestfd);

  rv = setgid(uid);
  if(rv == -1) {
    perror("setgid");
    return 1;
  }

  rv = setuid(uid);
  if(rv == -1) {
    perror("setuid");
    return 1;
  }

  /* extract from host into container's destination, as the user */
  rv = execl("/bin/tar", "tar", "xf", "-", NULL);
  if(rv == -1) {
    perror("execl");
    return 1;
  }

  // unreachable
  return 2;
}
