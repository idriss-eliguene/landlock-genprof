#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

static int read_path(const char *path) {
  int fd = open(path, O_RDONLY);
  if (fd < 0) {
    printf("path=%s operation=read result=failure errno=%d errno_name=%s\n",
           path, errno, strerror(errno));
    return 1;
  }
  char buf[128];
  ssize_t n = read(fd, buf, sizeof(buf));
  int saved = errno;
  close(fd);
  if (n < 0) {
    printf("path=%s operation=read result=failure errno=%d errno_name=%s\n",
           path, saved, strerror(saved));
    return 1;
  }
  printf("path=%s operation=read result=success errno=0\n", path);
  return 0;
}

int main(int argc, char **argv) {
  if (argc == 2 && strcmp(argv[1], "--loop") == 0) {
    for (;;) {
      (void)getpid();
      if (read_path("/data/allowed.txt") != 0) {
        return 1;
      }
      struct timespec pause = {.tv_sec = 1, .tv_nsec = 0};
      nanosleep(&pause, NULL);
    }
  }
  if (argc != 2) {
    fprintf(stderr, "usage: %s PATH | --loop\n", argv[0]);
    return 2;
  }
  return read_path(argv[1]);
}
