# How to Start — Student Onboarding Guide

> Project overview: [`README.md`](README.md) (English, reference documentation)
> or [`README.etudiants.md`](README.etudiants.md) (French).
> Version française : [`COMMENT_COMMENCER.md`](COMMENT_COMMENCER.md).

This guide is for the three students working on `landlock-genprof`. It
covers environment setup, understanding the existing code, and the
first concrete tasks per role.

---

## Table of contents

0. [Create your Ubuntu VM (Windows)](#0-create-your-ubuntu-vm-windows)
1. [Understand the project in 5 minutes](#1-understand-the-project-in-5-minutes)
2. [Set up your environment](#2-set-up-your-environment)
3. [Explore the existing code](#3-explore-the-existing-code)
4. [Git workflow](#4-git-workflow)
5. [First tasks per role](#5-first-tasks-per-role)
6. [Working without depending on others](#6-working-without-depending-on-others)
7. [Running CI locally](#7-running-ci-locally)
8. [Key concepts to understand before coding](#8-key-concepts-to-understand-before-coding)
9. [Frequently asked questions](#9-frequently-asked-questions)

---

## 0. Create your Ubuntu VM (Windows)

> This section is **for Windows students only**. Do it before anything
> else. If you're already on native Ubuntu 24.04, skip directly to
> [section 1](#1-understand-the-project-in-5-minutes).

Landlock and eBPF are **Linux kernel** features — they don't work
natively on Windows. You need an Ubuntu 24.04 VM (kernel 6.8) or 26.04
(kernel 7.0) — both are validated, see `README.md` §6. The steps below
use 24.04 as an example; just swap the ISO and the version selected in
VirtualBox/Hyper-V for 26.04 if that's what you're using — nothing else
changes.

Two options depending on your machine:

| Option | When to use it |
|---|---|
| **VirtualBox** | Windows 10/11 Home, or if Hyper-V is disabled |
| **Hyper-V** | Windows 10/11 Pro/Enterprise/Education (built into Windows) |

> **How do I know which to pick?** Press `Win`, type `winver`. If you
> see "Windows 11 Pro" or "Education" → Hyper-V. If you see "Home" →
> VirtualBox. **Don't enable both at the same time** (virtualization
> conflict).

---

### Option A — VirtualBox

#### 1. Download and install VirtualBox

1. Go to [virtualbox.org/wiki/Downloads](https://www.virtualbox.org/wiki/Downloads)
2. Download **VirtualBox 7.x — Windows hosts**
3. Run the installer and accept the default settings
4. Also install the **VirtualBox Extension Pack** (same page, same version)

#### 2. Download Ubuntu 24.04 LTS

1. Go to [ubuntu.com/download/desktop](https://ubuntu.com/download/desktop)
2. Download **Ubuntu 24.04 LTS** (`.iso` file, about 5 GB)
3. Keep the ISO accessible — you'll need it in the next step

#### 3. Create the VM

1. Open VirtualBox → **New**
2. Fill in:
   - Name: `ubuntu-landlock`
   - Type: `Linux` / Version: `Ubuntu 24.04 LTS (64-bit)`
3. RAM: **4,096 MB minimum** (8,192 MB recommended)
4. Hard disk: **Create a virtual hard disk now** → VDI → Dynamically
   allocated → size: **30 GB minimum**
5. Click **Create**

#### 4. Attach the ISO and boot

1. Select the VM → **Settings** → **Storage**
2. Under "IDE Controller", click the empty disk icon → **Choose a disk
   file** → select the downloaded Ubuntu ISO
3. **Settings** → **System** → **Processor** → check **Enable PAE/NX**,
   set **2 CPUs minimum**
4. **Settings** → **Display** → Video memory: **128 MB**
5. Start the VM → choose **Try or Install Ubuntu**
6. In the installer: choose **Minimal installation**, **Erase disk and
   install Ubuntu** (only affects the virtual disk — no risk to your
   Windows install)
7. Set a username and password → wait for the installation to finish
8. Reboot the VM when prompted → eject the ISO if asked (press Enter)

#### 5. Install Guest Additions (resolution + copy-paste)

Inside the Ubuntu VM, open a terminal:

```bash
sudo apt update && sudo apt install -y build-essential dkms linux-headers-$(uname -r)
```

In the VirtualBox menu: **Devices** → **Insert Guest Additions CD
image** → inside Ubuntu, mount the CD and double-click `autorun.sh`, or
from a terminal:

```bash
sudo /media/$USER/VBox_GAs_*/VBoxLinuxAdditions.run
```

Reboot the VM. You can now resize the window and copy-paste between
Windows and the VM.

#### 6. Check the kernel

```bash
uname -r   # should show 6.8.x-xx-generic
```

---

### Option B — Hyper-V (Windows 11 Pro / Enterprise / Education)

#### 1. Enable Hyper-V

Open **PowerShell as administrator**:

```powershell
Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All
```

Reboot when prompted. Check in the Start menu that **Hyper-V Manager**
is present.

> **Important:** once Hyper-V is enabled, VirtualBox no longer works
> correctly on the same machine. Don't enable both.

#### 2. Download Ubuntu 24.04 LTS

Go to [ubuntu.com/download/server](https://ubuntu.com/download/server)
and download **Ubuntu Server 24.04 LTS** (`.iso`).

> We use the **Server** edition (not Desktop) for Hyper-V because it's
> lighter and more stable with Hyper-V drivers. You can install a
> graphical interface afterward if needed, but a terminal is enough for
> this project.

#### 3. Create the VM in Hyper-V Manager

1. Open **Hyper-V Manager** → **Action** → **New** → **Virtual Machine**
2. Name: `ubuntu-landlock` → **Next**
3. Generation: **Generation 2** (UEFI, better performance) → **Next**
4. Startup memory: **4096 MB** (enable dynamic memory if you're limited)
5. Networking: select the **Default Switch virtual switch** → **Next**
6. Virtual hard disk: **30 GB minimum** → **Next**
7. Installation options: **Install an operating system from a bootable
   image file** → select the Ubuntu ISO → **Next** → **Finish**

#### 4. Configure before booting

In the VM's **Settings**:

- **Security** → uncheck **Secure Boot** (or select the "Microsoft UEFI
  Certificate Authority" template if Ubuntu doesn't boot)
- **Processor** → set **2 virtual processors minimum**

#### 5. Install Ubuntu

1. Boot the VM → choose the language → **Ubuntu Server (minimized)** or
   **Ubuntu Server**
2. Network configuration: leave the default (DHCP)
3. Disk: **Use an entire disk** → confirm
4. Profile: enter a username and password
5. **Install OpenSSH server** if offered (convenient for connecting from
   Windows Terminal)
6. Wait for it to finish → reboot

#### 6. SSH access from Windows (optional but recommended)

Once the VM has booted, get its IP from Hyper-V Manager (the "IP
Address" column) or from inside the VM:

```bash
ip addr show eth0 | grep 'inet '
```

From Windows Terminal:

```powershell
ssh youruser@<VM_IP>
```

You can work directly from Windows Terminal without going through the
Hyper-V window.

#### 7. Check the kernel

```bash
uname -r   # should show 6.8.x-xx-generic
```

---

### After creating the VM (common to VirtualBox and Hyper-V)

Update the system and install the project's dependencies:

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y git curl wget gcc build-essential linux-headers-$(uname -r)
```

The kernel check (`./hack/check-kernel.sh`) happens once the repo is
cloned — see [section 2, step 4](#2-set-up-your-environment).

You're ready to continue with [section 2](#2-set-up-your-environment).

---

### Docker — its exact role in this project

Docker is **already an implicit prerequisite**: `kind` (Kubernetes in
Docker) creates its K8s nodes as Docker containers. It must therefore be
installed on your Ubuntu VM.

```bash
# Install Docker on Ubuntu 24.04
sudo apt install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable" \
    | sudo tee /etc/apt/sources.list.d/docker.list
sudo apt update && sudo apt install -y docker-ce docker-ce-cli containerd.io
sudo usermod -aG docker $USER   # avoids needing sudo for every docker command
newgrp docker                   # applies it without logging out
docker version                  # verify
```

#### What Docker can and can NOT do for this project

| Use case | Docker alone | Ubuntu 24.04 VM |
|---|---|---|
| `go build ./...` | ✅ via `Dockerfile.dev` | ✅ |
| `go test -short ./...` (unit tests) | ✅ via `Dockerfile.dev` | ✅ |
| `go vet`, `gosec` | ✅ via `Dockerfile.dev` | ✅ |
| Bootstrapping `kind` | ✅ (that's its job) | ✅ |
| eBPF integration tests (Inspektor Gadget) | ❌ no BTF on WSL2 | ✅ |
| Landlock network (≥ 6.4) | ❌ WSL2 kernel ~5.15 | ✅ kernel 6.8 |
| `./hack/check-kernel.sh` fully green | ❌ | ✅ |

> **Summary:** Docker Desktop on Windows **does not replace** the
> Ubuntu VM, because the WSL2 kernel (~5.15) lacks
> `CONFIG_DEBUG_INFO_BTF` and doesn't support Landlock networking.
> Docker is useful **inside the VM** to run kind and for the fast-build
> `Dockerfile.dev`.

#### `Dockerfile.dev` — build and unit tests without a cluster

The repo has a `Dockerfile.dev` at the root to standardize the build
environment:

```bash
# From the repo root (in the VM or on native Linux)
docker build -f Dockerfile.dev -t landlock-genprof-dev .

# Build
docker run --rm landlock-genprof-dev go build ./...

# Unit tests (no eBPF kernel needed)
docker run --rm landlock-genprof-dev go test -short ./...

# Vet + gosec
docker run --rm landlock-genprof-dev sh -c "go vet ./... && gosec ./..."

# Interactive shell to explore
docker run --rm -it landlock-genprof-dev bash
```

Integration tests (which need a real kind cluster + eBPF) run directly
on the VM, not inside the container.

---

## 1. Understand the project in 5 minutes

**What we're building:** a Go command-line tool that observes a running
Kubernetes pod and automatically generates its Landlock security policy.

**Why it's useful:** hand-writing a Landlock policy means guessing in
advance every file and port an application will ever need. Forget
something → the app breaks in production. `landlock-genprof` observes
first, generates after.

**What we produce:** a YAML file (`LandlockProfile`) readable by PodLock
(a Kubernetes operator that enforces Landlock on pods).

**The full pipeline:**

```
running pod
        │
        ▼
  [Tracer] captures openat / connect / bind syscalls via eBPF
        │
        ▼
  [Synthesis] aggregates events → rules with a confidence level
        │
        ▼
  [YAML] generates a PodLock-compatible LandlockProfile
        │
        ▼
  human review → PodLock enforces the policy
```

**Current state of the code:** the skeleton is in place (Go types, repo
structure, CI). The critical functions still need implementing. Every
`panic("not implemented")` is a task for the team.

---

## 2. Set up your environment

### Step 1 — Configure SSH access to GitHub

Cloning the repo (next step) uses the `git@github.com:...` URL, i.e.
the **SSH protocol**, not HTTPS. An SSH key pair therefore needs to
exist on the machine (VM or native), with its public key registered on
your GitHub account, before you can clone.

#### How SSH works — in short

SSH (_Secure Shell_) authenticates your identity via **asymmetric
cryptography**: a pair of mathematically linked keys, generated
together.

- **Public key** (`id_ed25519.pub`): you give it to GitHub. It's only
  used to *verify* a signature — it's worthless to an attacker who only
  has it. It can be shared without risk (file, email, chat).
- **Private key** (`id_ed25519`, no extension): it stays **exclusively**
  on your machine. On every connection, your SSH client uses it to
  *sign* a cryptographic challenge GitHub sends; GitHub verifies that
  signature with your public key. The private key itself never travels
  over the network — only proof that it signs correctly is sent.

This is the reverse of a password: instead of sending a secret on every
connection (interceptable), you prove you know it without ever
revealing it.

#### Why the private key is critical

- **It *is* your identity.** Whoever gets your private key can
  impersonate you on GitHub — clone your private repos, push code in
  your name (including malicious code in a shared project like this
  one), read and modify everything your account has access to.
- **It doesn't expire and can't be revoked "remotely."** Unlike a
  password you can change instantly, a stolen private key stays valid
  until its matching public key is manually removed from GitHub
  (**Settings → SSH and GPG keys**) — an attacker can use it silently
  until you notice the compromise.
- **A compromise is hard to detect.** GitHub can't tell "you" apart
  from "someone who has your key" — SSH authentication succeeds either
  way.
- **That's why we protect it with a passphrase** (see below): even if
  the private key file is stolen (laptop theft, exfiltrated VM image,
  misconfigured backup), the passphrase prevents it from being
  immediately usable.

**Rules to follow:**
- **Never** commit a private key to a repo, even a private one.
- **Never** send it over chat, email, or paste it into a ticket.
- Don't store it in an unencrypted cloud-synced folder (Dropbox,
  Drive...).
- If you think it leaked: remove it from GitHub immediately
  (**Settings → SSH and GPG keys**) and generate a new pair.

#### 1. Check if a key already exists

```bash
ls -al ~/.ssh
# Look for files like id_ed25519 / id_ed25519.pub or id_rsa / id_rsa.pub
```

If a pair already exists and you know its passphrase, you can skip
directly to step 4 (adding it to the agent).

#### 2. Generate a new key pair

```bash
ssh-keygen -t ed25519 -C "your.email@example.com"
```

- `-t ed25519`: modern algorithm (elliptic curve), faster and safer
  than the old `rsa` for a much smaller key size. Only use `-t rsa -b
  4096` if a very old system requires it.
- `-C`: just a comment (often your email) to identify the key later in
  GitHub's list — no cryptographic value.

The prompt asks where to save it (leave the default path,
`~/.ssh/id_ed25519`, unless you have a specific need) then a
**passphrase**.

> **Set a passphrase.** It encrypts the private key on disk. Without
> one, anyone who copies the `id_ed25519` file can use it directly.
> With one, stealing the file alone isn't enough.

#### 3. Check the file permissions

SSH refuses to use a private key if its permissions are too open
(another user on the system could read it):

```bash
chmod 700 ~/.ssh
chmod 600 ~/.ssh/id_ed25519       # private key: read/write for you only
chmod 644 ~/.ssh/id_ed25519.pub   # public key: readable by everyone
```

#### 4. Add the key to the SSH agent (so you don't retype the passphrase)

```bash
eval "$(ssh-agent -s)"
ssh-add ~/.ssh/id_ed25519
```

`ssh-agent` keeps the key unlocked in memory for the session, after
you've entered the passphrase once.

#### 5. Add the public key to GitHub

```bash
cat ~/.ssh/id_ed25519.pub
```

Copy the full output (starts with `ssh-ed25519 AAAA...`), then on
GitHub: **Settings → SSH and GPG keys → New SSH key**, paste it, give
it a name (e.g. `vm-ubuntu-landlock`) and confirm.

#### 6. Test the connection

```bash
ssh -T git@github.com
```

Expected response:

```
Hi <your-username>! You've successfully authenticated, but GitHub does not
provide shell access.
```

That's normal — this message confirms authentication works. You can
now clone the repo in the next step.

---

### Step 2 — Clone the repo

```bash
git clone git@github.com:idriss-eliguene/landlock-genprof.git
cd landlock-genprof
```

### Step 3 — Install Go

```bash
# Check the installed version
go version   # should show go1.26 or later
```

If missing, install from [go.dev/dl](https://go.dev/dl/) — `amd64` and
`arm64` (common on a VM created from an Apple Silicon Mac) have
different archives, auto-detected below:

```bash
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

wget "https://go.dev/dl/go1.26.5.linux-${ARCH}.tar.gz"
sudo tar -C /usr/local -xzf "go1.26.5.linux-${ARCH}.tar.gz"
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

> ⚠️ If Go was already installed once with the wrong archive (`amd64`
> on an `arm64` VM, or the reverse), `go version` fails with `cannot
> execute binary file: Exec format error` instead of just showing the
> wrong version — remove `/usr/local/go` before reinstalling with the
> right archive: `sudo rm -rf /usr/local/go`.

### Step 4 — Check the kernel

Landlock and eBPF need a recent Linux kernel. **Ubuntu 24.04 is
recommended** (kernel 6.8 — covers everything).

```bash
./hack/check-kernel.sh
```

Expected output:

```
== Vérification du kernel ==
Kernel: 6.8.0-...
✅ Landlock FS supporté
✅ Landlock réseau supporté (>= 6.4)
✅ bpffs monté
```

> **On macOS:** Landlock and eBPF are Linux features. You need a Linux
> VM (UTM, Lima, or a cloud VM) to develop and test. Building and
> running unit tests without a kernel works on macOS.
>
> **On Windows:** see [section 0](#0-create-your-ubuntu-vm-windows)
> before continuing here.

### Step 5 — Build and test

```bash
# Build — must pass with no error
go build ./...
# equivalent: make build

# Unit tests (no cluster required)
go test ./...
# equivalent: make test

# Static check
go vet ./...
# equivalent: make vet
```

> On macOS/Windows, `internal/tracer.Trace()` compiles as a stub
> (a clear error instead of a real capture) — see `docs/architecture.md`
> §3. For a full `go build`/`go test` with the real `trace_linux.go`
> without the VM, use `make docker-test` (see §7 below).

### Step 6 — Install kind (local Kubernetes cluster)

kind (_Kubernetes IN Docker_) creates a local K8s cluster using Docker.
It shares the host kernel, which is essential for Landlock and eBPF to
work.

#### Recommended option: `./hack/init-vm.sh` (or `make init-vm`)

```bash
cd ~/landlock-genprof
git pull
./hack/init-vm.sh
# equivalent: make init-vm
```

`make help` lists the available shortcuts (`init-vm`, `check-kernel`) —
a `Makefile` at the repo root simply calls the scripts under `hack/`,
nothing more; use whichever form you prefer, both do exactly the same
thing.

This one command does everything below in this section **and** the
Inspektor Gadget/test pod part of section 5 (Student A):

| Script step | What it does | Why |
|---|---|---|
| 1/7 — kind | Installs the `kind` binary (pinned `v0.32.0`) | Creates a local K8s cluster that shares the VM's kernel |
| 2/7 — kubectl | Installs the `kubectl` binary (`v1.36.2`) | Command-line client to drive the cluster |
| 3/7 — Helm | Installs the `helm` binary (pinned `v4.2.3`) | Needed to install Cilium right after, and for the project's own Helm chart |
| 4/7 — kind cluster + Cilium | `kind create cluster` (default CNI disabled), then installs **Cilium** instead of kindnet | kindnet (kind's default CNI) doesn't support `NetworkPolicy` — without Cilium, `--network-out` would generate a file that never actually gets enforced, silently |
| 5/7 — Inspektor Gadget | Installs `ig` (standalone trace CLI) **and** `kubectl-gadget` (separate kubectl plugin) | Both are needed: `ig` traces locally, `kubectl gadget` deploys gadgets onto the cluster — see the note below |
| 6/7 — deployment | `kubectl gadget deploy`, then waits for the `gadget` namespace's pods to be `Ready` | Without this wait, you might think it's ready while the pods are still starting |
| 7/7 — test pod | Deploys `nginx-demo`, waits for it to be `Ready` | This is the target for the tracer's first tests (section 5) |

**Why two Inspektor Gadget binaries (`ig` and `kubectl-gadget`)?**
These are two distinct tools from the same project, and neither
replaces the other:
- `ig` traces syscalls **directly on the machine**, without going
  through Kubernetes — useful for quick debugging or outside a cluster.
- `kubectl-gadget` is a **kubectl plugin** (hence `kubectl gadget ...`,
  with a space, not a subcommand of `ig`) that deploys gadgets *inside*
  the cluster, as pods in the `gadget` namespace.

Installing only `ig` makes `kubectl gadget deploy` fail (command not
found) — an easy mistake when following partial documentation.

**Why the script is idempotent (safe to re-run):** every step starts by
checking whether its result already exists (`command -v kind`, `kind
get clusters`, `kubectl get pod nginx-demo`, ...) and skips work already
done. Concretely: if your network drops while downloading `ig`, or if
the `gadget` pods aren't `Ready` after 60s (`exit 1` with a helpful
message), you fix the reported problem and re-run **the same command**
— no need to start over from scratch or clean anything up by hand.

Expected final output:

```
✅ Infra prête. Premier test manuel :
    kubectl gadget run trace_open:latest -n default -c nginx-demo
  (dans un autre terminal : kubectl exec nginx-demo -- ls /etc)
```

> ⚠️ If the script stops at step 5/6 with `kubectl get pods -n gadget`
> not turning `Ready`, see the FAQ in section 9 ("the Inspektor Gadget
> SDK doesn't work on my kind cluster").

#### If you want the exact commands, or the script fails partway through

Deliberately **not** duplicated here as a copy-pasted block: the exact
install commands (versions, architecture handling, Cilium's Helm
values) live in exactly one place, `hack/init-vm.sh`, and a second
prose copy has already drifted out of sync with it once — a stale copy
that looks authoritative is worse than no copy. The script is short and
heavily commented; read it directly if you want to understand or replay
a specific step:

```bash
less hack/init-vm.sh
# or, to jump straight to the kind/kubectl/Helm/Cilium part:
sed -n '/3\/7/,/5\/7/p' hack/init-vm.sh
```

Each step is idempotent (checks whether its result already exists
before doing anything), so re-running `./hack/init-vm.sh` after fixing
a reported problem resumes where it left off — no need to run
individual steps by hand.

> ⚠️ **`kind: command not found` right after installing?** `go install`
> puts the binary in `$(go env GOPATH)/bin` (often `~/go/bin`), which
> isn't necessarily in your `PATH`. Add to `~/.bashrc`:
> `export PATH=$PATH:$(go env GOPATH)/bin`, then `source ~/.bashrc`.
> `./hack/init-vm.sh` detects and fixes this automatically for its own
> run (with a message reminding you to make it permanent yourself).

Expected output once the cluster itself is up (step 4/7):

```
NAME                        STATUS   ROLES           AGE
landlock-dev-control-plane  Ready    control-plane   30s
```

### Understanding what kind just created

When you run `kind create cluster`, kind creates **a single Docker
container** acting as a Kubernetes node. All the cluster's components
run inside that container. It's not a VM — it's Docker on your Linux
kernel, which is precisely why Landlock and eBPF work.

```
┌─────────────────────────────────────────────────────────┐
│  Docker container: landlock-dev-control-plane            │
│                                                         │
│  ┌─────────── Control Plane ───────────┐                │
│  │  kube-apiserver      ← entry point for the whole API │
│  │  etcd                ← the cluster's database        │
│  │  kube-scheduler      ← decides where to place pods   │
│  │  kube-controller-mgr ← maintains the desired state    │
│  └─────────────────────────────────────┘                │
│                                                         │
│  ┌─────────── Worker (same node) ──────┐                │
│  │  kubelet             ← agent that manages pods        │
│  │  kube-proxy          ← network rules (iptables)       │
│  │  containerd          ← runtime that starts pods        │
│  └─────────────────────────────────────┘                │
│                                                         │
│  ┌─────────── Add-ons ─────────────────┐                │
│  │  CoreDNS             ← cluster-internal DNS           │
│  │  Cilium              ← pod-to-pod networking (CNI)    │
│  └─────────────────────────────────────┘                │
└─────────────────────────────────────────────────────────┘
          │ shares the host kernel (Ubuntu 24.04 / 6.8)
```

#### Role of each component

| Component | Role | Why it matters for this project |
|---|---|---|
| **kube-apiserver** | Entry point for the whole K8s API | `client-go` (`internal/k8s/`) connects to it to resolve the target pod |
| **etcd** | Distributed key-value database | Stores cluster state (pods, namespaces...) — read via the apiserver, never directly |
| **kube-scheduler** | Chooses which node to place a pod on | Transparent to us — we never call it |
| **kube-controller-manager** | Reconciliation loop (ReplicaSet, etc.) | Transparent to us |
| **kubelet** | Per-node agent, starts/stops pods | It's what starts the container `landlock-genprof` will observe |
| **kube-proxy** | iptables/eBPF network rules | Transparent to us |
| **containerd** | Container runtime (replaces Docker inside K8s) | It creates the pod's namespace — Inspektor Gadget attaches its eBPF probes there |
| **CoreDNS** | Internal DNS (`nginx-demo.default.svc.cluster.local`) | Transparent for our tests, but needed by the cluster |
| **Cilium** | CNI — pod-to-pod networking, **and** `NetworkPolicy` enforcement | **Not transparent**: it's what actually enforces the `networkpolicy.yaml` generated by `--network-out`. `hack/init-vm.sh` installs Cilium instead of kind's default CNI (kindnet), which doesn't support `NetworkPolicy` at all — see [`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md) |

#### Commands to verify everything is healthy

Run these commands after `kind create cluster` to make sure the
cluster is operational before you start developing.

```bash
# 1. The node is ready (STATUS = Ready)
kubectl get nodes -o wide

# 2. All system pods are running (STATUS = Running)
kubectl get pods -n kube-system

# 3. Control plane components respond
kubectl get componentstatuses

# 4. The apiserver is reachable and authenticated
kubectl cluster-info

# 5. CoreDNS works (2 Running pods)
kubectl get pods -n kube-system -l k8s-app=kube-dns

# 6. Pod-to-pod networking works
kubectl run ping-test --image=busybox --restart=Never -- sleep 30
kubectl wait --for=condition=Ready pod/ping-test --timeout=30s
kubectl exec ping-test -- ping -c 2 8.8.8.8
kubectl delete pod ping-test
```

Expected output for `kubectl get pods -n kube-system`:

```
NAME                                              READY   STATUS    RESTARTS
cilium-xxxxx                                      1/1     Running   0
cilium-operator-xxxxx                             1/1     Running   0
coredns-7db6d8ff4d-xxxxx                          1/1     Running   0
coredns-7db6d8ff4d-yyyyy                          1/1     Running   0
etcd-landlock-dev-control-plane                   1/1     Running   0
kube-apiserver-landlock-dev-control-plane         1/1     Running   0
kube-controller-manager-landlock-dev-control-plane 1/1   Running   0
kube-proxy-xxxxx                                  1/1     Running   0
kube-scheduler-landlock-dev-control-plane         1/1     Running   0
```

> No `kube-proxy` or `kindnet` in some Cilium installs (Cilium can
> replace `kube-proxy` too) — `hack/init-vm.sh`'s default install keeps
> `kube-proxy` and just adds Cilium as the CNI, so both coexist as shown
> above.

> If a pod is `CrashLoopBackOff` or `Pending`, wait 60s and re-run
> `kubectl get pods -n kube-system`. kind sometimes needs a minute to
> fully start. If it persists:
>
> ```bash
> kubectl describe pod <pod-name> -n kube-system   # see events
> kubectl logs <pod-name> -n kube-system           # component logs
> ```

#### Everyday commands during development

```bash
# List pods in the default namespace (our test pods)
kubectl get pods

# Watch a pod's logs live
kubectl logs -f nginx-demo

# Open a shell inside a pod
kubectl exec -it nginx-demo -- sh

# See cluster events (scheduling errors, OOMKill...)
kubectl get events --sort-by=.lastTimestamp

# Cleanly delete and recreate the cluster (full reset)
kind delete cluster --name landlock-dev
kind create cluster --name landlock-dev
```

### Step 7 — Deploy a test pod (nginx)

```bash
kubectl run nginx-demo --image=nginx:alpine --port=80
kubectl wait --for=condition=Ready pod/nginx-demo --timeout=60s
kubectl get pod nginx-demo
```

This pod will be the target for the tracer's first tests.

### Step 8 — Apply the required manifests before the first `trace`

Since publishing a `SecurityProfileProposal` is mandatory, a first
`landlock-genprof trace` run fails if the CRDs/RBAC below aren't
already applied to the cluster.

```bash
# Tracer's base RBAC
kubectl apply -f deploy/rbac.yaml

# Mandatory SecurityProfileProposal publishing
kubectl apply -f deploy/crd-securityprofileproposal.yaml
kubectl apply -f deploy/rbac-proposal.yaml

# Required whenever a run composes securityContext data
# (very common in practice when syscalls are observed)
kubectl apply -f deploy/rbac-patched-manifest.yaml
```

Optional, depending on flags you use later:

```bash
# If you plan to use --history
kubectl apply -f deploy/crd-traininghistory.yaml
kubectl apply -f deploy/rbac-history.yaml

# If you plan to use --restart
kubectl apply -f deploy/rbac-restart.yaml
```

Alternative: instead of applying the files one by one, install
everything as a single Helm release — see
[`deploy/helm/landlock-genprof/README.md`](deploy/helm/landlock-genprof/README.md)
for the `restart.enabled`/`history.enabled` toggles:

```bash
helm install landlock-genprof deploy/helm/landlock-genprof
```

What you apply here lets you generate and publish profiles — it's not
enough to **enforce** them. See
[`docs/enforcement-prerequisites.md`](docs/enforcement-prerequisites.md)
for what else is needed (PodLock, security-profiles-operator), and its
known real limitation: PodLock doesn't work correctly on this reference
`kind` cluster, per its own documentation.

### Step 8bis — Run your first trace

Every example so far assumed `landlock-genprof` was already available
as `kubectl landlock-genprof` — a kubectl plugin is just an executable
named `kubectl-<name>` somewhere on your `PATH`, and nothing above
actually built or installed it yet. Do that now:

```bash
make install-plugin
kubectl plugin list | grep landlock-genprof   # sanity check
```

Then run an actual training run against the `nginx-demo` pod:

```bash
kubectl landlock-genprof trace \
  --pod nginx-demo \
  --namespace default \
  --binary /usr/sbin/nginx \
  --duration 60s \
  --out profile.yaml
```

See [`docs/usage.md`](docs/usage.md) for what each flag does — in
particular why `--binary` is required rather than auto-detected.

### Step 8ter — Proposal-first demo flow

Once a `trace` has run and the `SecurityProfileProposal` is published
in the cluster, you can rebuild the artifacts directly from that CRD
without asking the CLI to write files locally again.

```bash
# Export the proposal's artifacts into out/nginx-demo/
make export-proposal PROPOSAL=nginx-demo

# Prepare the demo: export + list artifacts + check the PodLock label
make demo-proposal PROPOSAL=nginx-demo

# Then apply the exported artifacts in the right order
make apply-proposal PROPOSAL=nginx-demo
```

Optional files missing from the proposal (e.g. NetworkPolicy or
SeccompProfile if nothing was generated this run) aren't kept in the
output folder.

---

## 3. Explore the existing code

Before writing a single line, read these files in order:

```
1. README.md                         → project overview (English;
                                        README.etudiants.md for the French version)
2. docs/roadmap.md                   → milestones and task assignment (English)
3. docs/threat-model.md              → attack surface (Student C) (English)
4. pkg/podlock/types.go              → output format (5 minutes)
5. internal/tracer/tracer.go         → Event and Options types (Student A)
6. internal/policy/synthesize.go     → Rule and Confidence types (Student B)
7. internal/k8s/target.go            → target pod resolution (Student B)
8. cmd/landlock-genprof/main.go      → CLI entry point (Student B)
9. examples/nginx-generated-profile.yaml  → concrete output format
```

**Command to explore quickly:**

```bash
# Read every Go file in the project
find . -name "*.go" | grep -v "_test.go" | sort | xargs head -40

# See the project's TODOs
grep -rn "TODO\|panic(\"not implemented\")" --include="*.go" .
```

Output of that last command, as it was **at the very start of the
project** (initial scaffolding, nothing implemented yet):

```
internal/k8s/target.go:      panic("not implemented")   ← M1, Student B
internal/policy/synthesize.go: panic("not implemented") ← M2, Student B
internal/tracer/tracer.go:   panic("not implemented")   ← M1, Student A
cmd/landlock-genprof/main.go: // TODO(M1): wire up ...   ← M1, Student B
```

Those four are now implemented (`Resolve()`, `Synthesize()`, `Trace()`
for `openat`, the CLI wired up with `cobra`). What's still open today if
you re-run the same command:

```
pkg/podlock/types.go:12: // TODO(M2): validate these types against PodLock's real schema
```

Also, not marked as a TODO in the code but still open per the roadmap
(`docs/roadmap.md`): `trace_tcpconnect`/`trace_bind` (network rights) in
`internal/tracer`, and the tracer's real minimal RBAC
(`ServiceAccount`/`Role`/`RoleBinding`, see `docs/threat-model.md`).

---

## 4. Git workflow

### Branches

```
master        → stable code, always buildable and testable
feat/tracer   → Student A (internal/tracer/)
feat/policy   → Student B (internal/policy/ + internal/k8s/ + cmd/)
feat/threat   → Student C (docs/ + CI)
```

### Enable the pre-commit hooks

Once per clone (not per branch):

```bash
git config core.hooksPath .githooks
```

Two hooks are enabled:

- **`pre-commit`**: runs `gofmt -l`, `go vet ./...`, `go build ./...`
  and `go test -cover ./...` before every commit — avoids pushing a
  commit that breaks CI over a trivial mistake (formatting, a
  compile-time typo). The coverage shown is informative, not
  blocking: most packages are still stubs with no tests. It doesn't
  reproduce `hack/check-kernel.sh`: this hook needs to stay runnable on
  macOS/Windows, whereas the kernel check only makes sense on the dev
  VM/Linux machine.
- **`commit-msg`**: rejects a commit if its message doesn't follow the
  `<type>(<scope>): <description>` convention (see §4 below —
  `feat`/`fix`/`docs`/`test`/`chore`).

### Start on your branch

```bash
# Student A
git checkout -b feat/tracer

# Student B
git checkout -b feat/policy

# Student C
git checkout -b feat/threat
```

### Daily work cycle

```bash
# 1. Fetch the latest changes from master
git fetch origin
git rebase origin/master

# 2. Work, commit regularly
git add internal/tracer/tracer.go
git commit -m "feat(tracer): add Trace() stub with Inspektor Gadget options"

# 3. Push your branch
git push origin feat/tracer

# 4. Open a Pull Request on GitHub once a milestone is reached
```

### Commit message convention

```
feat(tracer): short description
fix(policy): what got fixed
docs(threat-model): what was added
test(tracer): what got tested
chore(ci): CI update
```

### Absolute rule

**Never push directly to `master`.** Always go through a Pull Request
— even between students, even for a small change. This lets the
instructor track progress and lets the team review each other's work.

---

## 5. First tasks per role

### Student A — `internal/tracer/`

**M0 objective (weeks 1-2):** understand Inspektor Gadget and run an
existing gadget on the kind cluster.

> ⚠️ **If you find a tutorial or doc showing `ig trace open
> --containername ...`: that's outdated syntax.** Recent Inspektor
> Gadget versions (including `v0.54.1` used here for `ig`/
> `kubectl-gadget`) switched to an "image-based" gadget model — you run
> a gadget by its name and a tag (`trace_open:latest`) via `run`,
> rather than dedicated subcommands (`trace open`). It's `kubectl
> gadget run ...` you need here, since Inspektor Gadget is deployed *on
> the cluster* (`kubectl gadget deploy`), not just used locally.
>
> ⚠️ **Why `:latest` and not a pinned version, when everything else in
> this guide pins versions?** Gadget images (`trace_open`, `trace_exec`,
> ...) have their own release cycle, not synced with the `ig`/
> `kubectl-gadget` CLI releases — `trace_open:v0.54.1` doesn't exist
> (verified: the last real versioned tag for this gadget is `v0.27.0`).
> `:latest` is what the official documentation itself uses in all its
> examples; it's the safe value here.

Read the Inspektor Gadget documentation:
[inspektor-gadget.io/docs/latest](https://www.inspektor-gadget.io/docs/latest/).
`ig`/`kubectl-gadget` themselves are already installed and deployed if
you ran `./hack/init-vm.sh` in step 6 (its steps 5/7-6/7 — see that
script directly for the exact install commands, not duplicated here on
purpose, same reasoning as step 6's own note on this). Your first real
test:

```bash
# FIRST TEST — trace the nginx pod's openat calls
kubectl gadget run trace_open:latest -n default -c nginx-demo
# In another terminal: kubectl exec nginx-demo -- ls /etc
# Watch the events appear

# Same for executions (execve/execveat) — needed for
# LANDLOCK_ACCESS_FS_EXECUTE, see the note on --paths below
kubectl gadget run trace_exec:latest --paths -n default -c nginx-demo
```

**✅ Done for `openat` and `execve`**: `internal/tracer.Trace()` is no
longer a stub — see `internal/tracer/trace_linux.go`. It starts
`trace_open` **and** `trace_exec` concurrently via Inspektor Gadget's Go
SDK (gRPC runtime, against the DaemonSet already deployed on the
cluster), filters by `opts.Namespace`/`PodName`/`Container`, stops after
`opts.Duration` (`context.WithTimeout`), and merges both streams into a
single `[]Event`.

**Why two gadgets, not one:** `openat(2)` has no "exec" bit in its
flags (`O_ACCMODE` only distinguishes read/write/read_write — unlike
FreeBSD, Linux has no `O_EXEC`). `trace_open` alone can therefore never
know a path was *executed*; that signal only exists on
`execve(2)`/`execveat(2)`, which `trace_exec` observes directly (with
its `--paths` flag enabled, to get the path of the executed binary).
This gap was only discovered by testing for real on the cluster: see
`docs/policy-synthesis.md` for the full story of the bug
(`readExec`/`readWriteExec` were never reachable with real data until
this second gadget was wired in).

Important architectural point: this file has the `//go:build linux`
build tag — the Inspektor Gadget SDK doesn't compile at all on
macOS/Windows (it pulls in Linux-only code: eBPF, cgroups...).
`tracer.go` (the `Event`/`Options` types, without the SDK) stays
buildable everywhere; `trace_other.go` (`//go:build !linux`) returns a
clear error instead on other OSes. See `docs/architecture.md` §3 for
the full detail of this split and why it was necessary (not just a
style choice).

**Networking (`trace_tcpconnect`/`trace_bind`) is deliberately not
implemented**: PodLock's real CRD schema
(`github.com/flavio/podlock`) has no field to represent Landlock
network rights — verified directly in its source code, not assumed.
See `docs/roadmap.md` (M1) and `docs/policy-synthesis.md`.

The dependency is already in `go.mod` (pinned to `v0.54.1`, matching
the `ig`/`kubectl-gadget` binaries `hack/init-vm.sh` installs):

```bash
grep inspektor-gadget go.mod
```

---

### Student B — `cmd/` + `internal/k8s/` + `internal/policy/`

**M0 objective (weeks 1-2):** have a working CLI with cobra, and a
`Synthesize` function testable against mocked data.

**Task 1 — Replace the manual switch with cobra:**

```bash
go get github.com/spf13/cobra@latest
```

Target cobra structure for `cmd/landlock-genprof/main.go`:

```go
var rootCmd = &cobra.Command{Use: "landlock-genprof"}

var traceCmd = &cobra.Command{
    Use:   "trace",
    Short: "Starts a training run and generates a Landlock profile",
    RunE:  runTrace,
}

func init() {
    traceCmd.Flags().StringP("pod",       "p", "",    "Target pod name")
    traceCmd.Flags().StringP("namespace", "n", "default", "K8s namespace")
    traceCmd.Flags().DurationP("duration", "d", 60*time.Second, "Training run duration")
    traceCmd.Flags().StringP("out",       "o", "profile.yaml", "Output file")
    traceCmd.MarkFlagRequired("pod")
    rootCmd.AddCommand(traceCmd)
}
```

**Task 2 — Implement `Synthesize` against mocked data:**

Don't wait for the tracer. Create a test file with static events:

```go
// internal/policy/synthesize_test.go
func TestSynthesize_AggregatesByDirectory(t *testing.T) {
    events := []tracer.Event{
        {Syscall: "openat", Path: "/usr/share/nginx/index.html", Mode: "read"},
        {Syscall: "openat", Path: "/usr/share/nginx/css/style.css", Mode: "read"},
        {Syscall: "openat", Path: "/tmp/nginx.pid", Mode: "write"},
    }
    rules, err := Synthesize(events)
    // Expect: /usr/share/nginx → readOnly, /tmp → readWrite
    // No per-file rule — aggregation happens at the directory level
}
```

**Task 3 — Implement `Resolve` in `k8s/target.go`:**

```bash
go get k8s.io/client-go@latest
```

Use `client-go` to check that the pod exists before starting the
tracer.

---

### Student C — `docs/threat-model.md` + CI

**M0 objective (weeks 1-2):** complete the threat model with answers to
the open questions, and add `gosec` to CI.

**Task 1 — Complete `docs/threat-model.md`:**

Answer the open questions with real research:

```markdown
## 1. Capabilities required by the tracer

| Capability   | Why it's needed | Less permissive alternative |
|---|---|---|
| CAP_BPF      | Load an eBPF program | ... |
| CAP_SYS_ADMIN| perf_event_open access on kernels < 5.8 | ... |
```

Sources to consult:
- [Inspektor Gadget RBAC docs](https://www.inspektor-gadget.io/docs/latest/reference/rbac/)
- [Kubernetes Security Profiles Operator threat model](https://github.com/kubernetes-sigs/security-profiles-operator/blob/main/docs/threat-model.md)

**Task 2 — Add `gosec` to CI:**

Edit `.github/workflows/ci.yml`:

```yaml
- name: Security scan (gosec)
  uses: securego/gosec@master
  with:
    args: ./...
```

**Task 3 — Document the profile validation methodology:**

Answer in `docs/threat-model.md`:
- How many training runs do we recommend before trusting a profile?
- What minimal test scenarios (startup, HTTP request, 404 error, config
  reload) cover an nginx's frequent code paths?
- How do we detect that a `low confidence` profile caused a production
  regression?

---

## 6. Working without depending on others

Decoupling the roles is deliberate. Here's how to make progress without
waiting.

### Student B — without Student A's tracer

Define a mock function in the tests:

```go
// internal/policy/testdata_test.go
func mockNginxEvents() []tracer.Event {
    return []tracer.Event{
        {Syscall: "openat", Path: "/usr/sbin/nginx",           Mode: "exec"},
        {Syscall: "openat", Path: "/etc/nginx/nginx.conf",     Mode: "read"},
        {Syscall: "openat", Path: "/usr/share/nginx/html/index.html", Mode: "read"},
        {Syscall: "openat", Path: "/var/log/nginx/access.log", Mode: "write"},
        {Syscall: "openat", Path: "/tmp/nginx.pid",            Mode: "write"},
        {Syscall: "connect", Port: 80,                          Mode: "read"},
    }
}
```

Develop and test `Synthesize` entirely against this data. Integration
with the real tracer happens at M1 — the `[]Event` interface is shared.

### Student C — without the application code

The threat model and CI can be developed independently of the Go code.
CI (`go build ./...`, `go vet ./...`) already works on the scaffolding.
`gosec` can be added now — it'll find little to scan for now, but the
setup will be in place for the following milestones.

### Student A — without the rest of the CLI

The tracer can be developed and tested in isolation, without the CLI:

```go
// internal/tracer/tracer_test.go (integration test, needs the cluster)
//go:build integration

func TestTrace_OpenAt(t *testing.T) {
    events, err := Trace(Options{
        PodName:   "nginx-demo",
        Namespace: "default",
        Duration:  10 * time.Second,
    })
    require.NoError(t, err)
    openatEvents := filterBySyscall(events, "openat")
    assert.NotEmpty(t, openatEvents, "no openat captured — the tracer isn't working")
}
```

```bash
# Run only the integration tests (with an active kind cluster)
go test -tags integration ./internal/tracer/
```

---

## 7. Running CI locally

Reproduce exactly what GitHub Actions will run (on the Linux VM — see
the `make docker-test` note below for macOS/Windows):

```bash
# 1. Check kernel prerequisites
./hack/check-kernel.sh   # or: make check-kernel

# 2. Build
go build ./...           # or: make build

# 3. Tests (verbose + coverage, as in CI)
go test -v -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
# equivalent (without the per-function detail): make test

# 4. Vet
go vet ./...              # or: make vet

# 5. SAST — gosec (separate "security" job in CI, pinned version)
go install github.com/securego/gosec/v2/cmd/gosec@v2.28.0
gosec ./...

# 6. SCA — Trivy (Go dependencies / go.sum, needs Trivy installed locally)
# https://aquasecurity.github.io/trivy/latest/getting-started/installation/
trivy fs --scanners vuln --severity CRITICAL,HIGH .
```

> 💡 **On macOS/Windows**, `make docker-test` does build + vet + test
> inside a Linux container (`Dockerfile.dev`) — the only way to exercise
> the real `internal/tracer/trace_linux.go` (not the stub) without the
> VM. Still no real cluster/eBPF inside it (`Dockerfile.dev`
> deliberately stops at build/vet/test), so it doesn't replace
> `hack/init-vm.sh` for testing `Trace()` under real conditions.

**Rule:** CI must pass on `master` at all times. If you break the
build, that's your top priority before any other task. The `security`
job (steps 5-6) doesn't block merges yet (see `docs/threat-model.md`
§4) but is worth running locally before pushing.

---

## 8. Key concepts to understand before coding

### Landlock

Landlock is an LSM (_Linux Security Module_) that lets a **process
confine itself** without root privileges. Once armed, the process can
only access the paths and ports it explicitly declared.

Essential reading:
- [Landlock — official page](https://landlock.io/)
- [`man 7 landlock`](https://man7.org/linux/man-pages/man7/landlock.7.html)
- [LWN.net article on Landlock](https://lwn.net/Articles/859908/) — historical context

### eBPF and Inspektor Gadget

eBPF lets you run code directly inside the Linux kernel, without
modifying its source. It's the technology used to observe a pod's
syscalls without instrumenting it.

**Inspektor Gadget** provides ready-made eBPF gadgets (no need to write
eBPF from scratch) and a Go SDK to consume them.

Reading:
- [eBPF in 10 minutes](https://ebpf.io/what-is-ebpf/) — accessible introduction
- [Inspektor Gadget quickstart](https://www.inspektor-gadget.io/docs/latest/quick-start/)
- [trace_open gadget](https://www.inspektor-gadget.io/docs/latest/gadgets/trace_open/)

### PodLock and the LandlockProfile CRD

PodLock is a Kubernetes operator (Kubewarden ecosystem) that enforces
Landlock profiles on pods. We generate YAML files compatible with its
`LandlockProfile` CRD.

Reading:
- [PodLock on GitHub](https://github.com/flavio/podlock) — read the
  README and the CRD examples

### client-go

`client-go` is the official Go library for talking to the Kubernetes
API. It's used in `internal/k8s/target.go` to check that a pod exists
before starting the tracer.

Reading:
- [client-go examples](https://github.com/kubernetes/client-go/tree/master/examples)

---

## 9. Frequently asked questions

**Q: I don't have a Linux machine, what do I do?**

Two options:
- UTM (Apple Silicon macOS) or VirtualBox (Intel) with Ubuntu 24.04
- A free cloud VM (GitHub Codespaces, GCP Free Tier, Oracle Cloud Free Tier)

Building and unit tests (`go build`, `go test`) work on macOS or
Windows. Only integration tests (tracer + kind cluster) need Linux.

---

**Q: `go build ./...` fails with import errors.**

Normal in M0: the real dependencies aren't in `go.mod` yet. That's the
first M0 task — adding the `go get`s for Inspektor Gadget, client-go,
and sigs.k8s.io/yaml.

---

**Q: How do I know if my commit will break CI?**

Run the steps from [section 7 — Running CI locally](#7-running-ci-locally)
before pushing.

---

**Q: Student A — the Inspektor Gadget SDK doesn't work on my kind cluster.**

Check that Inspektor Gadget is actually deployed on the cluster:

```bash
kubectl gadget deploy
kubectl get pods -n gadget
```

If the gadget pods don't start, check the logs:

```bash
kubectl logs -n gadget -l app=gadget
```

The most common cause: the host kernel doesn't support the BPF ring
buffer (kernel < 5.8). On Ubuntu 24.04, this isn't an issue.

---

**Q: What's the difference between plan A (Inspektor Gadget) and plan B (`strace`)?**

| | Plan A — Inspektor Gadget | Plan B — strace |
|---|---|---|
| Technology | eBPF (kernel) | ptrace |
| Overhead | Very low | Significant (ptrace blocks on every syscall) |
| Kernel requirement | ≥ 5.8 | Available everywhere |
| Implementation | Go SDK → `Trace()` API | `strace -f -e trace=openat,...` + parsing |
| `Event{}` interface | **Identical** | **Identical** |

If plan B is activated in week 3-4, only `internal/tracer/tracer.go`
changes. The rest of the pipeline (synthesis, YAML, CLI) doesn't
change.

---

**Q: Where do I ask questions?**

Open a GitHub issue in the repo with the appropriate label:
- `question/tracer` — Student A
- `question/policy` — Student B
- `question/threat` — Student C
- `question/setup` — environment problem (everyone)
