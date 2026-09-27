#define _GNU_SOURCE
#include <arpa/inet.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <stdio.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

int main(void) {
  const struct timespec pause = {.tv_sec = 0, .tv_nsec = 250000000};
  for (;;) {
    char buffer[128];
    int file = open("/etc/hostname", O_RDONLY | O_CLOEXEC);
    if (file >= 0) {
      (void)read(file, buffer, sizeof(buffer));
      close(file);
    }
    int raw = socket(AF_INET, SOCK_RAW, IPPROTO_ICMP);
    if (raw >= 0) close(raw);
    nanosleep(&pause, NULL);
  }
}
