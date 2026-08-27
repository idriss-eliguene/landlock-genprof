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

static void probe_exit(int status) {
	/* The recorded workload observes SYS_exit, while exit_group may be
	 * absent from a least-privilege profile.  Terminate through the raw
	 * syscall so reporting a denied probe does not require an unobserved
	 * libc shutdown path. */
	syscall(SYS_exit, status);
	for (;;) {
		/* SYS_exit must not return; keep the compiler from treating this as
		 * reachable if a restrictive profile returns an error. */
		sleep(1);
	}
}

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

	/* The syscall result is domain data (getpriority may legitimately be
	 * non-zero).  Process success is determined only by errno. */
	int ok = errno == 0;
	/* Use an unbuffered write so the diagnostic survives an expected non-zero
	 * exec status and is authoritative for the harness. */
	const char *errno_name = errno == EPERM ? "Operation not permitted" :
	                         (errno == 0 ? "Success" : "Unknown error");
	dprintf(STDOUT_FILENO,
	         "syscall=%s result=%ld errno=%d errno_name=%s status=%s\n",
	         argv[1], result, errno, errno_name,
	         ok ? "success" : "denied");
	probe_exit(ok ? 0 : 1);
}
