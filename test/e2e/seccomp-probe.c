// Minimal syscall probe for the real-node SPO enforcement E2E.

#define _GNU_SOURCE

#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <sys/resource.h>
#include <sys/syscall.h>
#include <sys/sysinfo.h>
#include <sys/utsname.h>
#include <unistd.h>

int main(int argc, char **argv) {
	long result;

	if (argc != 2) {
		fprintf(stderr, "usage: seccomp-probe <getpid|getpriority|sysinfo|uname|sched_yield>\n");
		return 2;
	}

	errno = 0;
	if (strcmp(argv[1], "getpid") == 0) {
		result = syscall(SYS_getpid);
	} else if (strcmp(argv[1], "getpriority") == 0) {
		result = syscall(SYS_getpriority, PRIO_PROCESS, 0);
	} else if (strcmp(argv[1], "sysinfo") == 0) {
		struct sysinfo info;
		result = syscall(SYS_sysinfo, &info);
	} else if (strcmp(argv[1], "uname") == 0) {
		struct utsname name;
		result = syscall(SYS_uname, &name);
	} else if (strcmp(argv[1], "sched_yield") == 0) {
		result = syscall(SYS_sched_yield);
	} else {
		fprintf(stderr, "unsupported syscall: %s\n", argv[1]);
		return 2;
	}

	printf("syscall=%s result=%ld errno=%d\n", argv[1], result, errno);
	return errno == 0 ? 0 : 1;
}
