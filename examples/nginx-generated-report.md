<!--
Real output of one live `trace` run against nginx-demo on linux/arm64
(issue #94), same run as the other nginx-generated-* files except
nginx-generated-profile.yaml. Capture environment, image digest, traffic
and exact commands: examples/README.md. Links below point to the file
names the run wrote (nginx-demo-*), kept as generated.

WARNING, ARM64 seccomp: the seccomp architecture list in this run is
wrong for AArch64. Inspektor Gadget v0.55.1's advise_seccomp hardcodes
SCMP_ARCH_X86_64/SCMP_ARCH_X86/SCMP_ARCH_X32 whatever the node is, and
landlock-genprof keeps what the gadget reports. Do not use these seccomp
profiles for ARM64 enforcement.
-->

# Security Profile Review — nginx-demo

- **Generated:** 2026-09-28T10:53:10+02:00
- **Namespace/Container:** default/nginx-demo
- **Binary:** /usr/sbin/nginx
- **Training duration:** 1m0s
- **--history used:** no — Confidence below is internal/policy's single-run proxy, not a real cross-run ratio

## Filesystem

See [`nginx-demo-profile.yaml`](nginx-demo-profile.yaml) for the full PodLock profile.

| Path | Permissions | Confidence |
|---|---|---|
| `/etc` | read | high |
| `/etc/nginx` | read | medium |
| `/etc/nginx/conf.d` | read | medium |
| `/etc/ssl` | read | low |
| `/run` | read, write | low |
| `/usr/lib` | read | high |
| `/usr/sbin` | execute | low |
| `/usr/share/nginx` | read | high |
| `/var/log/nginx` | write | high |

## Network

See [`nginx-demo-networkpolicy.yaml`](nginx-demo-networkpolicy.yaml) for the full NetworkPolicy.

| Port | Direction | Confidence |
|---|---|---|
| 80 | ingress | medium |

## Syscalls

See [`operator/lg-v1-nginx-demo-c7ccca0069549f09.json`](operator/lg-v1-nginx-demo-c7ccca0069549f09.json) for the full seccomp profile.

⚠ Confidence reflects only this run without `--history` — see `docs/policy-synthesis.md`'s "Syscall aggregation" section.

**high (12):** `accept4`, `close`, `epoll_ctl`, `epoll_pwait`, `fstat`, `gettid`, `newfstatat`, `openat`, `recvfrom`, `sendfile`, `write`, `writev`

**low (70):** `bind`, `brk`, `clone`, `connect`, `copy_file_range`, `dup`, `dup3`, `epoll_create1`, `eventfd2`, `execve`, `exit_group`, `faccessat`, `fadvise64`, `fchmod`, `fchown`, `fchownat`, `fcntl`, `fstatfs`, `futex`, `getcwd`, `getdents64`, `getegid`, `geteuid`, `getgid`, `getpgid`, `getpid`, `getppid`, `getrandom`, `getuid`, `io_setup`, `ioctl`, `listen`, `lseek`, `mkdirat`, `mmap`, `mount_setattr`, `move_mount`, `mprotect`, `munmap`, `open_tree`, `pipe2`, `ppoll`, `prctl`, `pread64`, `prlimit64`, `pwritev2`, `read`, `readlinkat`, `recvmsg`, `renameat`, `rseq`, `rt_sigaction`, `rt_sigprocmask`, `rt_sigreturn`, `rt_sigsuspend`, `sched_getaffinity`, `sendmsg`, `set_robust_list`, `set_tid_address`, `setgid`, `setgroups`, `setsockopt`, `setuid`, `socket`, `socketpair`, `statfs`, `umask`, `uname`, `utimensat`, `wait4`

## Capabilities

See [`nginx-demo-capabilities.yaml`](nginx-demo-capabilities.yaml).

See [`nginx-demo-securitycontext.yaml`](nginx-demo-securitycontext.yaml).

| Capability | Confidence |
|---|---|
| `CAP_CHOWN` | high |
| `CAP_DAC_OVERRIDE` | medium |
| `CAP_SETGID` | high |
| `CAP_SETUID` | high |
| `CAP_SYS_ADMIN` | high |

## Review checklist

- [ ] Re-run with `--history` before trusting any `low`/`medium` entry (docs/policy-synthesis.md).
- [ ] Review every `low`/`medium` entry before enforcing anything.
- [ ] Seccomp/capabilities are fatal if wrong — review with extra care.
- [ ] See docs/threat-model.md for the full validation methodology.
