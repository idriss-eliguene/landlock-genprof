#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char **argv) {
  if (argc != 2) {
    fprintf(stderr, "usage: %s PATH\n", argv[0]);
    return 2;
  }
  int fd = open(argv[1], O_RDONLY);
  if (fd < 0) {
    printf("path=%s operation=read result=failure errno=%d errno_name=%s\n",
           argv[1], errno, strerror(errno));
    return 1;
  }
  char buf[128];
  ssize_t n = read(fd, buf, sizeof(buf));
  int saved = errno;
  close(fd);
  if (n < 0) {
    printf("path=%s operation=read result=failure errno=%d errno_name=%s\n",
           argv[1], saved, strerror(saved));
    return 1;
  }
  printf("path=%s operation=read result=success errno=0\n", argv[1]);
  return 0;
}
