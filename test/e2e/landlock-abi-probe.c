#define _GNU_SOURCE
#include <errno.h>
#include <linux/landlock.h>
#include <stdio.h>
#include <sys/syscall.h>
#include <unistd.h>

int main(void) {
    errno = 0;
    long abi = syscall(SYS_landlock_create_ruleset, NULL, 0,
                       LANDLOCK_CREATE_RULESET_VERSION);
    if (abi < 0) {
        perror("landlock_create_ruleset");
        return 1;
    }
    printf("LANDLOCK_ABI=%ld\n", abi);
    return abi >= 3 ? 0 : 2;
}
