#ifndef MSG_H
#define MSG_H 1

#define MSG_VERSION 1

#include <sys/time.h>
#include <sys/resource.h>

#include "pwd.h"

typedef struct msg__array_s msg__array_t;
typedef struct msg__rlimit_s msg__rlimit_t;
typedef struct msg__user_s msg__user_t;
typedef struct msg__dir_s msg__dir_t;
typedef union msg_u msg_t;
typedef struct msg_request_s msg_request_t;
typedef struct msg_signal_s msg_signal_t;
typedef struct msg_response_s msg_response_t;

struct msg__array_s {
  int count;
  char buf[8 * 1024];
};

struct msg__rlimit_s {
  int count;
  struct {
    int id;
    struct rlimit rlim;
  } rlim[RLIMIT_NLIMITS];
};

struct msg__user_s {
  char name[32];
};

struct msg__dir_s {
  char path[1024];
};

enum msg_type_e {
    MSG_TYPE_REQ,
    MSG_TYPE_SIG
};

#define MSG_S_HEAD int version; \
    enum msg_type_e type;

struct msg_s {
    MSG_S_HEAD
};

struct msg_signal_s {
    MSG_S_HEAD
    int signal;
    int pid;
};

struct msg_request_s {
  MSG_S_HEAD
  int tty;
  msg__array_t arg;
  msg__array_t env;
  msg__rlimit_t rlim;
  msg__user_t user;
  msg__dir_t dir;
};

union msg_u {
    struct msg_request_s req;
    struct msg_signal_s sig;
};

struct msg_response_s {
  int version;
};

int msg_array_import(msg__array_t * a, int count, const char ** ptr);
const char ** msg_array_export(msg__array_t * a);

int msg_rlimit_import(msg__rlimit_t *);
int msg_rlimit_export(msg__rlimit_t *);

int msg_user_import(msg__user_t *u, const char *name);
int msg_user_export(msg__user_t *u, struct passwd *pw);

int msg_dir_import(msg__dir_t *d, const char *dir);

void msg_signal_init(msg_signal_t *sig);
void msg_request_init(msg_request_t *req);
void msg_response_init(msg_response_t *res);

#endif
