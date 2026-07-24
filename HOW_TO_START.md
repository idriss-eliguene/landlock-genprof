# Comment démarrer — Guide d'onboarding étudiant

> Vision globale du projet : [`README.etudiants.md`](README.etudiants.md) (français)
> ou [`README.md`](README.md) (anglais, documentation de référence).

Ce guide est destiné aux trois étudiants qui travaillent sur `landlock-genprof`.
Il couvre la mise en place de l'environnement, la compréhension du code existant,
et les premières tâches concrètes par rôle.

---

## Sommaire

0. [Créer sa VM Ubuntu (Windows)](#0-créer-sa-vm-ubuntu-windows)
1. [Comprendre le projet en 5 minutes](#1-comprendre-le-projet-en-5-minutes)
2. [Mettre en place l'environnement](#2-mettre-en-place-lenvironnement)
3. [Explorer le code existant](#3-explorer-le-code-existant)
4. [Workflow Git](#4-workflow-git)
5. [Premières tâches par rôle](#5-premières-tâches-par-rôle)
6. [Travailler sans dépendre des autres](#6-travailler-sans-dépendre-des-autres)
7. [Lancer la CI en local](#7-lancer-la-ci-en-local)
8. [Concepts clés à comprendre avant de coder](#8-concepts-clés-à-comprendre-avant-de-coder)
9. [Questions fréquentes](#9-questions-fréquentes)

---

## 0. Créer sa VM Ubuntu (Windows)

> Cette section est **uniquement pour les étudiants sur Windows**. Elle est à faire
> avant tout le reste. Si tu es déjà sur Ubuntu 24.04 natif, passe directement à
> la [section 1](#1-comprendre-le-projet-en-5-minutes).

Landlock et eBPF sont des fonctionnalités du **noyau Linux** — ils ne fonctionnent
pas nativement sur Windows. Il faut une VM Ubuntu 24.04 (kernel 6.8).

Deux options selon ta machine :

| Option | Quand l'utiliser |
|---|---|
| **VirtualBox** | Windows 10/11 Home, ou si Hyper-V est désactivé |
| **Hyper-V** | Windows 10/11 Pro/Enterprise/Education (intégré à Windows) |

> **Comment savoir lequel choisir ?** Touche `Win`, tape `winver`. Si tu vois
> "Windows 11 Pro" ou "Education" → Hyper-V. Si tu vois "Home" → VirtualBox.
> **Ne pas activer les deux en même temps** (conflit de virtualisation).

---

### Option A — VirtualBox

#### 1. Télécharger et installer VirtualBox

1. Va sur [virtualbox.org/wiki/Downloads](https://www.virtualbox.org/wiki/Downloads)
2. Télécharge **VirtualBox 7.x — Windows hosts**
3. Lance l'installeur et accepte les paramètres par défaut
4. Installe aussi le **VirtualBox Extension Pack** (même page, même version)

#### 2. Télécharger Ubuntu 24.04 LTS

1. Va sur [ubuntu.com/download/desktop](https://ubuntu.com/download/desktop)
2. Télécharge **Ubuntu 24.04 LTS** (fichier `.iso`, environ 5 Go)
3. Garde l'ISO accessible — tu en auras besoin à l'étape suivante

#### 3. Créer la VM

1. Ouvre VirtualBox → **Nouvelle**
2. Remplis :
   - Nom : `ubuntu-landlock`
   - Type : `Linux` / Version : `Ubuntu 24.04 LTS (64-bit)`
3. Mémoire RAM : **4 096 Mo minimum** (8 192 Mo recommandé)
4. Disque dur : **Créer un disque virtuel maintenant** → VDI → Dynamiquement alloué
   → taille : **30 Go minimum**
5. Clique sur **Créer**

#### 4. Rattacher l'ISO et démarrer

1. Sélectionne la VM → **Paramètres** → **Stockage**
2. Sous "Contrôleur IDE", clique sur l'icône disque vide → **Choisir un fichier de
   disque optique** → sélectionne l'ISO Ubuntu téléchargé
3. **Paramètres** → **Système** → **Processeur** → cocher **Activer PAE/NX**,
   mettre **2 CPU minimum**
4. **Paramètres** → **Affichage** → Mémoire vidéo : **128 Mo**
5. Démarre la VM → choisis **Try or Install Ubuntu**
6. Dans l'installeur : choisis **Installation minimale**, **Effacer le disque et
   installer Ubuntu** (ne concerne que le disque virtuel — aucun risque pour ton
   Windows)
7. Définis un nom d'utilisateur et mot de passe → attends la fin de l'installation
8. Redémarre la VM quand demandé → éjecte l'ISO si demandé (appuie sur Entrée)

#### 5. Installer les Guest Additions (résolution + copier-coller)

Dans la VM Ubuntu, ouvre un terminal :

```bash
sudo apt update && sudo apt install -y build-essential dkms linux-headers-$(uname -r)
```

Dans le menu VirtualBox : **Périphériques** → **Insérer l'image du CD des Additions
invités** → dans Ubuntu, monte le CD et double-clique sur `autorun.sh`, ou en
terminal :

```bash
sudo /media/$USER/VBox_GAs_*/VBoxLinuxAdditions.run
```

Redémarre la VM. Tu peux maintenant redimensionner la fenêtre et faire
copier-coller entre Windows et la VM.

#### 6. Vérifier le kernel

```bash
uname -r   # doit afficher 6.8.x-xx-generic
```

---

### Option B — Hyper-V (Windows 11 Pro / Enterprise / Education)

#### 1. Activer Hyper-V

Ouvre **PowerShell en administrateur** :

```powershell
Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All
```

Redémarre quand demandé. Vérifie dans le menu Démarrer que **Hyper-V Manager**
est présent.

> **Important :** une fois Hyper-V activé, VirtualBox ne fonctionne plus
> correctement sur la même machine. Ne pas activer les deux.

#### 2. Télécharger Ubuntu 24.04 LTS

Va sur [ubuntu.com/download/server](https://ubuntu.com/download/server) et
télécharge **Ubuntu Server 24.04 LTS** (`.iso`).

> On prend la version **Server** (pas Desktop) pour Hyper-V car elle est plus
> légère et stable avec les drivers Hyper-V. On peut installer une interface
> graphique ensuite si besoin, mais pour ce projet un terminal suffit.

#### 3. Créer la VM dans Hyper-V Manager

1. Ouvre **Hyper-V Manager** → **Action** → **Nouvelle** → **Machine virtuelle**
2. Nom : `ubuntu-landlock` → **Suivant**
3. Génération : **Génération 2** (UEFI, meilleures performances) → **Suivant**
4. Mémoire de démarrage : **4096 Mo** (active la mémoire dynamique si tu es limité)
5. Réseau : sélectionne le **commutateur virtuel Default Switch** → **Suivant**
6. Disque dur virtuel : **30 Go minimum** → **Suivant**
7. Options d'installation : **Installer un système d'exploitation depuis un fichier
   image de démarrage** → sélectionne l'ISO Ubuntu → **Suivant** → **Terminer**

#### 4. Configurer avant de démarrer

Dans **Paramètres** de la VM :

- **Sécurité** → décocher **Démarrage sécurisé** (ou choisir le modèle
  "Microsoft UEFI Certificate Authority" si Ubuntu ne démarre pas)
- **Processeur** → mettre **2 processeurs virtuels minimum**

#### 5. Installer Ubuntu

1. Démarre la VM → choisis la langue → **Ubuntu Server (minimized)** ou
   **Ubuntu Server**
2. Configuration réseau : laisser par défaut (DHCP)
3. Disque : **Use an entire disk** → confirme
4. Profil : entre un nom d'utilisateur et mot de passe
5. **Installe OpenSSH server** si proposé (pratique pour se connecter depuis
   Windows Terminal)
6. Attends la fin → redémarre

#### 6. Accès SSH depuis Windows (optionnel mais recommandé)

Une fois la VM démarrée, récupère son IP dans Hyper-V Manager (colonne "IP Address")
ou dans la VM :

```bash
ip addr show eth0 | grep 'inet '
```

Depuis Windows Terminal :

```powershell
ssh tonuser@<IP_de_la_VM>
```

Tu peux travailler directement depuis Windows Terminal sans passer par la fenêtre
Hyper-V.

#### 7. Vérifier le kernel

```bash
uname -r   # doit afficher 6.8.x-xx-generic
```

---

### Après la création de la VM (commun VirtualBox et Hyper-V)

Mets à jour le système et installe les dépendances du projet :

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y git curl wget gcc build-essential linux-headers-$(uname -r)
```

La vérification du kernel (`./hack/check-kernel.sh`) se fait une fois le repo
cloné — voir [section 2, étape 4](#2-mettre-en-place-lenvironnement).

Tu es prêt·e à continuer avec la [section 2](#2-mettre-en-place-lenvironnement).

---

### Docker — rôle exact dans ce projet

Docker est **déjà un prérequis implicite** : `kind` (Kubernetes in Docker) crée ses
nœuds K8s comme des conteneurs Docker. Il doit donc être installé sur ta VM Ubuntu.

```bash
# Installer Docker sur Ubuntu 24.04
sudo apt install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable" \
    | sudo tee /etc/apt/sources.list.d/docker.list
sudo apt update && sudo apt install -y docker-ce docker-ce-cli containerd.io
sudo usermod -aG docker $USER   # évite d'utiliser sudo à chaque commande docker
newgrp docker                   # applique sans déconnexion
docker version                  # vérifie
```

#### Ce que Docker peut et ne peut PAS faire pour ce projet

| Usage | Docker seul | VM Ubuntu 24.04 |
|---|---|---|
| `go build ./...` | ✅ via `Dockerfile.dev` | ✅ |
| `go test -short ./...` (tests unitaires) | ✅ via `Dockerfile.dev` | ✅ |
| `go vet`, `gosec` | ✅ via `Dockerfile.dev` | ✅ |
| Bootstrapper `kind` | ✅ (c'est son rôle) | ✅ |
| Tests d'intégration eBPF (Inspektor Gadget) | ❌ BTF absent sur WSL2 | ✅ |
| Landlock réseau (≥ 6.4) | ❌ kernel WSL2 ~5.15 | ✅ kernel 6.8 |
| `./hack/check-kernel.sh` vert complet | ❌ | ✅ |

> **Résumé :** Docker Desktop sur Windows **ne remplace pas** la VM Ubuntu car le
> kernel WSL2 (~5.15) n'a pas `CONFIG_DEBUG_INFO_BTF` et ne supporte pas
> Landlock réseau. Docker est utile **à l'intérieur de la VM** pour faire tourner
> kind et pour le `Dockerfile.dev` de build rapide.

#### `Dockerfile.dev` — build et tests unitaires sans cluster

Le repo contient un `Dockerfile.dev` à la racine pour standardiser l'environnement
de compilation :

```bash
# Depuis la racine du repo (dans la VM ou sur Linux natif)
docker build -f Dockerfile.dev -t landlock-genprof-dev .

# Build
docker run --rm landlock-genprof-dev go build ./...

# Tests unitaires (pas besoin de kernel eBPF)
docker run --rm landlock-genprof-dev go test -short ./...

# Vet + gosec
docker run --rm landlock-genprof-dev sh -c "go vet ./... && gosec ./..."

# Shell interactif pour explorer
docker run --rm -it landlock-genprof-dev bash
```

Les tests d'intégration (qui nécessitent un vrai cluster kind + eBPF) se lancent
directement sur la VM, pas dans le conteneur.

---

## 1. Comprendre le projet en 5 minutes

**Ce qu'on construit :** un outil en ligne de commande Go qui observe un pod
Kubernetes en fonctionnement et génère automatiquement sa policy de sécurité
Landlock.

**Pourquoi c'est utile :** écrire une policy Landlock à la main, c'est deviner
à l'avance tous les fichiers et ports qu'une application va utiliser. Si on oublie
quelque chose → l'appli casse en prod. `landlock-genprof` observe d'abord, génère
ensuite.

**Ce qu'on produit :** un fichier YAML (`LandlockProfile`) lisible par PodLock
(un opérateur Kubernetes qui applique Landlock sur les pods).

**Le pipeline complet :**

```
pod en fonctionnement
        │
        ▼
  [Tracer] capture les syscalls openat / connect / bind via eBPF
        │
        ▼
  [Synthèse] agrège les événements → règles avec niveau de confiance
        │
        ▼
  [YAML] génère un LandlockProfile compatible PodLock
        │
        ▼
  revue humaine → PodLock applique la policy
```

**L'état actuel du code :** le squelette est en place (types Go, structure du repo,
CI). Les fonctions critiques sont à implémenter. Chaque `panic("not implemented")`
est une tâche pour l'équipe.

---

## 2. Mettre en place l'environnement

### Étape 1 — Configurer l'accès SSH à GitHub

Le clone du repo (étape suivante) utilise l'URL `git@github.com:...`, c'est-à-dire
le **protocole SSH**, pas HTTPS. Il faut donc qu'une paire de clés SSH existe sur
la machine (VM ou natif) et que sa clé publique soit enregistrée sur ton compte
GitHub, avant de pouvoir cloner.

#### Comment fonctionne SSH — en bref

SSH (_Secure Shell_) authentifie ton identité par **cryptographie asymétrique** :
une paire de clés mathématiquement liées, générées ensemble.

- **Clé publique** (`id_ed25519.pub`) : tu la donnes à GitHub. Elle sert uniquement
  à *vérifier* une signature — elle est sans valeur pour un attaquant qui ne
  possède qu'elle. Elle peut être partagée sans risque (fichier, email, chat).
- **Clé privée** (`id_ed25519`, sans extension) : elle reste **exclusivement** sur
  ta machine. À chaque connexion, ton client SSH l'utilise pour *signer* un défi
  cryptographique que GitHub envoie ; GitHub vérifie cette signature avec ta clé
  publique. La clé privée elle-même ne transite jamais sur le réseau — seule la
  preuve qu'elle signe correctement est envoyée.

C'est l'inverse d'un mot de passe : au lieu d'envoyer un secret à chaque connexion
(donc interceptable), tu prouves que tu le connais sans jamais le révéler.

#### Pourquoi la clé privée est critique

- **Elle *est* ton identité.** Quiconque obtient ta clé privée peut se faire passer
  pour toi sur GitHub — cloner tes repos privés, pousser du code en ton nom (y
  compris du code malveillant dans un projet partagé comme celui-ci), lire et
  modifier tout ce à quoi ton compte a accès.
- **Elle n'expire pas et n'est pas révocable "à distance".** Contrairement à un mot
  de passe qu'on peut changer instantanément, une clé privée volée reste valable
  tant que sa clé publique correspondante n'a pas été supprimée manuellement de
  GitHub (**Settings → SSH and GPG keys**) — un attaquant peut l'utiliser en
  silence jusqu'à ce que tu remarques la compromission.
- **Une compromission est difficile à détecter.** GitHub ne peut pas distinguer
  "toi" de "quelqu'un qui possède ta clé" — l'authentification SSH réussit dans
  les deux cas.
- **C'est pour ça qu'on la protège par une passphrase** (voir plus bas) : même si
  le fichier de la clé privée est volé (vol de laptop, image de VM exfiltrée,
  backup mal configuré), la passphrase empêche qu'il soit immédiatement
  utilisable.

**Règles à respecter :**
- Ne **jamais** committer une clé privée dans un repo, même privé.
- Ne **jamais** l'envoyer par chat, email ou la coller dans un ticket.
- Ne pas la stocker dans un dossier synchronisé cloud non chiffré (Dropbox, Drive…).
- Si tu penses qu'elle a fuité : supprime-la immédiatement de GitHub
  (**Settings → SSH and GPG keys**) et régénère une nouvelle paire.

#### 1. Vérifier si une clé existe déjà

```bash
ls -al ~/.ssh
# Cherche des fichiers comme id_ed25519 / id_ed25519.pub ou id_rsa / id_rsa.pub
```

Si une paire existe déjà et que tu en connais la passphrase, tu peux passer
directement à l'étape 4 (l'ajouter à l'agent).

#### 2. Générer une nouvelle paire de clés

```bash
ssh-keygen -t ed25519 -C "ton.email@example.com"
```

- `-t ed25519` : algorithme moderne (courbe elliptique), plus rapide et plus
  sûr que le vieux `rsa` pour une taille de clé bien plus petite. Utilise
  `-t rsa -b 4096` uniquement si un système très ancien l'exige.
- `-C` : juste un commentaire (souvent ton email) pour identifier la clé
  plus tard dans la liste GitHub — n'a aucune valeur cryptographique.

Le programme demande où sauvegarder (laisse le chemin par défaut,
`~/.ssh/id_ed25519`, sauf besoin spécifique) puis une **passphrase**.

> **Mets une passphrase.** Elle chiffre la clé privée sur disque. Sans elle,
> quiconque copie le fichier `id_ed25519` peut l'utiliser directement. Avec
> elle, un vol du fichier seul ne suffit pas.

#### 3. Vérifier les permissions du fichier

SSH refuse d'utiliser une clé privée si ses permissions sont trop larges
(un autre utilisateur du système pourrait la lire) :

```bash
chmod 700 ~/.ssh
chmod 600 ~/.ssh/id_ed25519       # clé privée : lecture/écriture pour toi seul
chmod 644 ~/.ssh/id_ed25519.pub   # clé publique : peut être lue par tous
```

#### 4. Ajouter la clé à l'agent SSH (pour ne pas retaper la passphrase)

```bash
eval "$(ssh-agent -s)"
ssh-add ~/.ssh/id_ed25519
```

`ssh-agent` garde la clé déverrouillée en mémoire pour la session, après avoir
saisi la passphrase une fois.

#### 5. Ajouter la clé publique à GitHub

```bash
cat ~/.ssh/id_ed25519.pub
```

Copie la sortie complète (commence par `ssh-ed25519 AAAA...`), puis sur GitHub :
**Settings → SSH and GPG keys → New SSH key**, colle-la, donne-lui un nom
(ex. `vm-ubuntu-landlock`) et valide.

#### 6. Tester la connexion

```bash
ssh -T git@github.com
```

Réponse attendue :

```
Hi <ton-username>! You've successfully authenticated, but GitHub does not
provide shell access.
```

C'est normal — ce message confirme que l'authentification fonctionne. Tu peux
maintenant cloner le repo à l'étape suivante.

---

### Étape 2 — Cloner le repo

```bash
git clone git@github.com:idriss-eliguene/landlock-genprof.git
cd landlock-genprof
```

### Étape 3 — Installer Go

```bash
# Vérifier la version installée
go version   # doit afficher go1.26 ou supérieur

# Si absent, installer depuis https://go.dev/dl/
# Sur Ubuntu :
wget https://go.dev/dl/go1.26.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### Étape 4 — Vérifier le kernel

Landlock et eBPF nécessitent un kernel Linux récent. **Ubuntu 24.04 est recommandé**
(kernel 6.8 — couvre tout).

```bash
./hack/check-kernel.sh
```

Sortie attendue :

```
== Vérification du kernel ==
Kernel: 6.8.0-...
✅ Landlock FS supporté
✅ Landlock réseau supporté (>= 6.4)
✅ bpffs monté
```

> **Sur macOS :** Landlock et eBPF sont des fonctionnalités Linux. Il faut une VM
> Linux (UTM, Lima, ou une VM cloud) pour développer et tester. Le build et les
> tests unitaires sans kernel fonctionnent sur macOS.
>
> **Sur Windows :** voir la [section 0](#0-créer-sa-vm-ubuntu-windows) avant de
> continuer ici.

### Étape 5 — Builder et tester

```bash
# Build — doit passer sans erreur
go build ./...
# équivalent : make build

# Tests unitaires (pas de cluster requis)
go test ./...
# équivalent : make test

# Vérification statique
go vet ./...
# équivalent : make vet
```

> Sur macOS/Windows, `internal/tracer.Trace()` compile en version stub
> (erreur claire au lieu d'une vraie capture) — voir `docs/architecture.md`
> §3. Pour un `go build`/`go test` complet avec le vrai `trace_linux.go`
> sans la VM, utilise `make docker-test` (voir §7 ci-dessous).

### Étape 6 — Installer kind (cluster Kubernetes local)

kind (_Kubernetes IN Docker_) crée un cluster K8s local en utilisant Docker.
Il partage le kernel hôte, ce qui est indispensable pour que Landlock et eBPF
fonctionnent.

#### Option recommandée : `./hack/init-vm.sh` (ou `make init-vm`)

```bash
cd ~/landlock-genprof
git pull
./hack/init-vm.sh
# équivalent : make init-vm
```

`make help` liste les raccourcis disponibles (`init-vm`, `check-kernel`) —
un `Makefile` à la racine du repo appelle simplement les scripts de `hack/`,
rien de plus ; utilise la forme que tu préfères, les deux font exactement
la même chose.

Cette seule commande fait tout ce qui suit dans cette section **et** la
partie Inspektor Gadget/pod de test de la section 5 (Étudiant A) :

| Étape du script | Ce qu'elle fait | Pourquoi |
|---|---|---|
| 1/6 — kind | Installe le binaire `kind` (version figée `v0.32.0`) | Crée un cluster K8s local qui partage le kernel de la VM |
| 2/6 — kubectl | Installe le binaire `kubectl` (`v1.36.2`) | Client en ligne de commande pour piloter le cluster |
| 3/6 — cluster kind | `kind create cluster --name landlock-dev` | Le cluster K8s local lui-même (un conteneur Docker, voir plus bas) |
| 4/6 — Inspektor Gadget | Installe `ig` (CLI de trace autonome) **et** `kubectl-gadget` (plugin kubectl séparé) | Les deux sont nécessaires : `ig` sert à tracer en local, `kubectl gadget` à déployer les gadgets sur le cluster — voir la remarque plus bas |
| 5/6 — déploiement | `kubectl gadget deploy`, puis attend que les pods du namespace `gadget` soient `Ready` | Sans cette attente, tu peux croire que c'est prêt alors que les pods sont encore en train de démarrer |
| 6/6 — pod de test | Déploie `nginx-demo`, attend qu'il soit `Ready` | C'est la cible des premiers tests du tracer (section 5) |

**Pourquoi deux binaires Inspektor Gadget (`ig` et `kubectl-gadget`) ?**
Ce sont deux outils distincts du même projet, qui ne se remplacent pas :
- `ig` trace des syscalls **directement sur la machine**, sans passer par
  Kubernetes — utile pour du debug rapide ou hors cluster.
- `kubectl-gadget` est un **plugin kubectl** (d'où `kubectl gadget ...`,
  avec un espace, pas un sous-programme de `ig`) qui déploie les gadgets
  *dans* le cluster, sous forme de pods dans le namespace `gadget`.

Installer seulement `ig` fait échouer `kubectl gadget deploy` (commande
introuvable) — c'est une erreur facile à faire en suivant une doc partielle.

**Pourquoi le script est idempotent (relançable sans risque) :** chaque
étape commence par vérifier si le résultat existe déjà (`command -v kind`,
`kind get clusters`, `kubectl get pod nginx-demo`, ...) et saute le travail
déjà fait. Concrètement : si ta connexion réseau coupe pendant le
téléchargement d'`ig`, ou si les pods `gadget` ne sont pas encore `Ready`
au bout de 60s (`exit 1` avec un message d'aide), tu corriges le problème
signalé et tu relances **la même commande** — pas besoin de tout
recommencer à zéro ni de nettoyer quoi que ce soit à la main.

Sortie finale attendue :

```
✅ Infra prête. Premier test manuel :
    kubectl gadget run trace_open:latest -n default -c nginx-demo
  (dans un autre terminal : kubectl exec nginx-demo -- ls /etc)
```

> ⚠️ Si le script s'arrête à l'étape 5/6 avec `kubectl get pods -n gadget`
> qui ne passe pas à `Ready`, voir la FAQ en section 9
> (« le SDK Inspektor Gadget ne fonctionne pas sur mon cluster kind »).

#### Comprendre chaque étape en détail (si tu préfères, ou si le script échoue)

Les commandes ci-dessous font exactement ce que fait `./hack/init-vm.sh`
pour kind et kubectl — utile pour comprendre pas à pas plutôt que lancer
une boîte noire, ou pour rejouer une étape précise si le script s'arrête
en cours de route.

> ⚠️ **Vérifie ton architecture avant de copier-coller** (`uname -m`) :
> `x86_64` → remplace `<ARCH>` par `amd64` ci-dessous ; `aarch64`/`arm64`
> (fréquent sur VM créée depuis un Mac Apple Silicon) → remplace par
> `arm64`. `./hack/init-vm.sh` le fait automatiquement pour toi — c'est
> justement pour éviter cette manipulation manuelle qu'il existe.

```bash
# Installer kind (version figée, pas @latest)
go install sigs.k8s.io/kind@v0.32.0

# Installer kubectl (version figée, pas @latest — remplace <ARCH>, voir ci-dessus)
curl -LO "https://dl.k8s.io/release/v1.36.2/bin/linux/<ARCH>/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
rm kubectl
kubectl version --client

# Créer le cluster de dev
kind create cluster --name landlock-dev

# Vérifier
kubectl cluster-info --context kind-landlock-dev
kubectl get nodes
```

> ⚠️ **`kind: command not found` juste après l'installation ?** `go install`
> place le binaire dans `$(go env GOPATH)/bin` (souvent `~/go/bin`), qui
> n'est pas forcément dans ton `PATH`. Ajoute à `~/.bashrc` :
> `export PATH=$PATH:$(go env GOPATH)/bin`, puis `source ~/.bashrc`.
> `./hack/init-vm.sh` détecte et corrige ça automatiquement pour sa propre
> exécution (avec un message qui te rappelle de le faire toi-même de façon
> permanente).

Sortie attendue :

```
NAME                        STATUS   ROLES           AGE
landlock-dev-control-plane  Ready    control-plane   30s
```

### Comprendre ce que kind vient de créer

Quand tu lances `kind create cluster`, kind crée **un seul conteneur Docker** qui
joue le rôle de nœud Kubernetes. À l'intérieur de ce conteneur tournent tous les
composants du cluster. Ce n'est pas une VM — c'est du Docker sur ton kernel Linux,
ce qui est précisément pourquoi Landlock et eBPF fonctionnent.

```
┌─────────────────────────────────────────────────────────┐
│  Conteneur Docker : landlock-dev-control-plane          │
│                                                         │
│  ┌─────────── Control Plane ───────────┐                │
│  │  kube-apiserver      ← point d'entrée de toute API  │
│  │  etcd                ← base de données du cluster   │
│  │  kube-scheduler      ← décide où placer les pods    │
│  │  kube-controller-mgr ← maintient l'état désiré      │
│  └─────────────────────────────────────┘                │
│                                                         │
│  ┌─────────── Worker (même nœud) ──────┐                │
│  │  kubelet             ← agent qui gère les pods       │
│  │  kube-proxy          ← règles réseau (iptables)      │
│  │  containerd          ← runtime qui démarre les pods  │
│  └─────────────────────────────────────┘                │
│                                                         │
│  ┌─────────── Add-ons ─────────────────┐                │
│  │  CoreDNS             ← DNS interne du cluster        │
│  │  kindnet             ← réseau entre pods (CNI)       │
│  └─────────────────────────────────────┘                │
└─────────────────────────────────────────────────────────┘
          │ partage le kernel hôte (Ubuntu 24.04 / 6.8)
```

#### Rôle de chaque composant

| Composant | Rôle | Pourquoi ça compte pour ce projet |
|---|---|---|
| **kube-apiserver** | Point d'entrée de toute l'API K8s | `client-go` (section `internal/k8s/`) s'y connecte pour résoudre le pod cible |
| **etcd** | Base de données clé-valeur distribuée | Stocke l'état du cluster (pods, namespaces…) — lu via l'apiserver, jamais directement |
| **kube-scheduler** | Choisit le nœud où placer un pod | Transparent pour nous — on ne l'appelle pas |
| **kube-controller-manager** | Boucle de réconciliation (ReplicaSet, etc.) | Transparent pour nous |
| **kubelet** | Agent sur chaque nœud, démarre/arrête les pods | C'est lui qui démarre le conteneur que `landlock-genprof` va observer |
| **kube-proxy** | Règles réseau iptables/eBPF | Transparent pour nous |
| **containerd** | Runtime de conteneurs (remplace Docker dans K8s) | C'est lui qui crée le namespace du pod — Inspektor Gadget y attache ses sondes eBPF |
| **CoreDNS** | DNS interne (`nginx-demo.default.svc.cluster.local`) | Transparent pour nos tests, mais nécessaire au cluster |
| **kindnet** | CNI — réseau entre pods | Transparent pour nous |

#### Commandes pour vérifier que tout est sain

Lance ces commandes après `kind create cluster` pour t'assurer que le cluster est
opérationnel avant de commencer à développer.

```bash
# 1. Le nœud est prêt (STATUS = Ready)
kubectl get nodes -o wide

# 2. Tous les pods système tournent (STATUS = Running)
kubectl get pods -n kube-system

# 3. Les composants du control plane répondent
kubectl get componentstatuses

# 4. L'apiserver est joignable et authentifié
kubectl cluster-info

# 5. CoreDNS fonctionne (2 pods Running)
kubectl get pods -n kube-system -l k8s-app=kube-dns

# 6. Le réseau inter-pods est opérationnel
kubectl run ping-test --image=busybox --restart=Never -- sleep 30
kubectl wait --for=condition=Ready pod/ping-test --timeout=30s
kubectl exec ping-test -- ping -c 2 8.8.8.8
kubectl delete pod ping-test
```

Sortie attendue pour `kubectl get pods -n kube-system` :

```
NAME                                              READY   STATUS    RESTARTS
coredns-7db6d8ff4d-xxxxx                          1/1     Running   0
coredns-7db6d8ff4d-yyyyy                          1/1     Running   0
etcd-landlock-dev-control-plane                   1/1     Running   0
kindnet-xxxxx                                     1/1     Running   0
kube-apiserver-landlock-dev-control-plane         1/1     Running   0
kube-controller-manager-landlock-dev-control-plane 1/1   Running   0
kube-proxy-xxxxx                                  1/1     Running   0
kube-scheduler-landlock-dev-control-plane         1/1     Running   0
```

> Si un pod est en `CrashLoopBackOff` ou `Pending`, attends 60 s et relance
> `kubectl get pods -n kube-system`. kind a parfois besoin d'une minute pour
> tout démarrer. Si ça persiste :
>
> ```bash
> kubectl describe pod <nom-du-pod> -n kube-system   # voir les événements
> kubectl logs <nom-du-pod> -n kube-system           # logs du composant
> ```

#### Commandes du quotidien pendant le développement

```bash
# Lister les pods de l'espace de noms default (nos pods de test)
kubectl get pods

# Voir les logs d'un pod en temps réel
kubectl logs -f nginx-demo

# Ouvrir un shell dans un pod
kubectl exec -it nginx-demo -- sh

# Voir les événements du cluster (erreurs de scheduling, OOMKill…)
kubectl get events --sort-by=.lastTimestamp

# Supprimer et recréer proprement le cluster (reset complet)
kind delete cluster --name landlock-dev
kind create cluster --name landlock-dev
```

### Étape 7 — Déployer un pod de test (nginx)

```bash
kubectl run nginx-demo --image=nginx:alpine --port=80
kubectl wait --for=condition=Ready pod/nginx-demo --timeout=60s
kubectl get pod nginx-demo
```

Ce pod sera la cible des premiers tests du tracer.

### Étape 8 — Appliquer les manifests requis avant le premier `trace`

Depuis que la publication `SecurityProfileProposal` est obligatoire, un premier
run `landlock-genprof trace` échoue si les CRD/RBAC ci-dessous ne sont pas déjà
appliqués au cluster.

```bash
# RBAC de base du tracer
kubectl apply -f deploy/rbac.yaml

# Publication obligatoire de SecurityProfileProposal
kubectl apply -f deploy/crd-securityprofileproposal.yaml
kubectl apply -f deploy/rbac-proposal.yaml

# Requis dès qu'un run compose des données securityContext
# (très fréquent en pratique quand des syscalls sont observés)
kubectl apply -f deploy/rbac-patched-manifest.yaml
```

Optionnel selon les flags utilisés plus tard :

```bash
# Si tu comptes utiliser --history
kubectl apply -f deploy/crd-traininghistory.yaml
kubectl apply -f deploy/rbac-history.yaml

# Si tu comptes utiliser --restart
kubectl apply -f deploy/rbac-restart.yaml
```

Alternative : au lieu d'appliquer les fichiers un par un, installer tout
en une seule release Helm — voir
[`deploy/helm/landlock-genprof/README.md`](deploy/helm/landlock-genprof/README.md)
pour le détail des toggles `restart.enabled`/`history.enabled` :

```bash
helm install landlock-genprof deploy/helm/landlock-genprof
```

### Étape 8bis — Flux de démo proposal-first

Une fois un `trace` exécuté et la `SecurityProfileProposal` publiée dans le
cluster, tu peux reconstruire les artefacts directement depuis cette CRD sans
redemander au CLI d'écrire les fichiers localement.

```bash
# Exporte les artefacts de la proposal dans out/nginx-demo/
make export-proposal PROPOSAL=nginx-demo

# Prépare la démo : export + liste des artefacts + vérification du label PodLock
make demo-proposal PROPOSAL=nginx-demo

# Applique ensuite les artefacts exportés dans le bon ordre
make apply-proposal PROPOSAL=nginx-demo
```

Les fichiers optionnels absents de la proposal (par exemple NetworkPolicy ou
SeccompProfile si rien n'a été généré sur ce run) ne sont pas conservés dans le
dossier de sortie.

---

## 3. Explorer le code existant

Avant d'écrire la moindre ligne, lire ces fichiers dans l'ordre :

```
1. README.md                         → vision globale du projet (anglais ;
                                        README.etudiants.md pour la version française)
2. docs/roadmap.md                   → jalons et répartition (anglais)
3. docs/threat-model.md              → surface d'attaque (Étudiante C) (anglais)
4. pkg/podlock/types.go              → format de sortie (5 minutes)
5. internal/tracer/tracer.go         → types Event et Options (Étudiant A)
6. internal/policy/synthesize.go     → types Rule et Confidence (Étudiant B)
7. internal/k8s/target.go            → résolution du pod cible (Étudiant B)
8. cmd/landlock-genprof/main.go      → point d'entrée CLI (Étudiant B)
9. examples/nginx-generated-profile.yaml  → format de sortie concret
```

**Commande pour explorer rapidement :**

```bash
# Lire tous les fichiers Go du projet
find . -name "*.go" | grep -v "_test.go" | sort | xargs head -40

# Voir les TODO du projet
grep -rn "TODO\|panic(\"not implemented\")" --include="*.go" .
```

Sortie de la dernière commande, telle qu'elle était **au tout début du
projet** (scaffolding initial, rien d'implémenté) :

```
internal/k8s/target.go:      panic("not implemented")   ← M1, Étudiant B
internal/policy/synthesize.go: panic("not implemented") ← M2, Étudiant B
internal/tracer/tracer.go:   panic("not implemented")   ← M1, Étudiant A
cmd/landlock-genprof/main.go: // TODO(M1): brancher ... ← M1, Étudiant B
```

Ces quatre-là sont maintenant implémentés (`Resolve()`, `Synthesize()`,
`Trace()` pour `openat`, le CLI câblé avec `cobra`). Ce qui reste
aujourd'hui si tu relances la même commande :

```
pkg/podlock/types.go:12: // TODO(M2): valider ces types face au schéma réel de PodLock
```

Plus, pas marqué en TODO dans le code mais toujours ouvert d'après le
roadmap (`docs/roadmap.md`) : `trace_tcpconnect`/`trace_bind` (droits
réseau) dans `internal/tracer`, et le RBAC minimal réel du tracer
(`ServiceAccount`/`Role`/`RoleBinding`, voir `docs/threat-model.md`).

---

## 4. Workflow Git

### Branches

```
master        → code stable, toujours buildable et testable
feat/tracer   → Étudiant A (internal/tracer/)
feat/policy   → Étudiant B (internal/policy/ + internal/k8s/ + cmd/)
feat/threat   → Étudiante C (docs/ + CI)
```

### Activer les pre-commit hooks

Une fois par clone (pas par branche) :

```bash
git config core.hooksPath .githooks
```

Deux hooks sont activés :

- **`pre-commit`** : lance `gofmt -l`, `go vet ./...`, `go build ./...` et
  `go test -cover ./...` avant chaque commit — ça évite de pousser un
  commit qui casse la CI pour une erreur triviale (formatage, typo de
  compilation). La couverture affichée est informative, pas bloquante :
  la plupart des packages sont encore des stubs sans aucun test. Il ne
  reproduit pas `hack/check-kernel.sh` : ce hook doit rester exécutable sur
  macOS/Windows, alors que la vérification kernel n'a de sens que sur la
  VM/machine Linux de dev.
- **`commit-msg`** : rejette un commit si son message ne respecte pas la
  convention `<type>(<scope>): <description>` (voir §4 ci-dessous —
  `feat`/`fix`/`docs`/`test`/`chore`).

### Démarrer sur sa branche

```bash
# Étudiant A
git checkout -b feat/tracer

# Étudiant B
git checkout -b feat/policy

# Étudiante C
git checkout -b feat/threat
```

### Cycle de travail quotidien

```bash
# 1. Récupérer les dernières modifications de master
git fetch origin
git rebase origin/master

# 2. Travailler, committer régulièrement
git add internal/tracer/tracer.go
git commit -m "feat(tracer): add Trace() stub with Inspektor Gadget options"

# 3. Pousser sa branche
git push origin feat/tracer

# 4. Ouvrir une Pull Request sur GitHub quand un jalon est atteint
```

### Convention de messages de commit

```
feat(tracer): description courte
fix(policy): ce qui est corrigé
docs(threat-model): ce qui est ajouté
test(tracer): ce qui est testé
chore(ci): mise à jour de la CI
```

### Règle absolue

**Ne jamais pousser directement sur `master`.** Toujours passer par une Pull Request
— même entre étudiants, même pour une petite modification. Ça permet à l'enseignant
de suivre l'avancement et à l'équipe de se relire.

---

## 5. Premières tâches par rôle

### Étudiant A — `internal/tracer/`

**Objectif M0 (semaine 1-2) :** comprendre Inspektor Gadget et faire tourner
un gadget existant sur le cluster kind.

> ⚠️ Remplace `<ARCH>` par `amd64` ou `arm64` selon `uname -m` (voir la
> remarque de l'étape 6) — `./hack/init-vm.sh` s'en charge automatiquement.
>
> ⚠️ **Si tu trouves un tutoriel ou une doc qui montre `ig trace open
> --containername ...` : c'est une syntaxe obsolète.** Les versions
> récentes d'Inspektor Gadget (dont `v0.54.1` utilisée ici pour `ig`/
> `kubectl-gadget`) sont passées à un modèle de gadgets "à la image" — on
> lance un gadget par son nom et un tag (`trace_open:latest`) via `run`,
> plutôt que par des sous-commandes dédiées (`trace open`). C'est
> `kubectl gadget run ...` qu'il faut utiliser ici, puisque Inspektor
> Gadget est déployé *sur le cluster* (`kubectl gadget deploy`), pas juste
> utilisé en local.
>
> ⚠️ **Pourquoi `:latest` et pas une version figée, alors que tout le reste
> de ce guide fige les versions ?** Les images de gadgets (`trace_open`,
> `trace_exec`, ...) ont leur propre cycle de publication, pas synchronisé
> avec les releases du CLI `ig`/`kubectl-gadget` — `trace_open:v0.54.1`
> n'existe pas (vérifié : le dernier tag versionné réel de ce gadget est
> `v0.27.0`). `:latest` est ce que la documentation officielle utilise
> elle-même dans tous ses exemples ; c'est la valeur sûre ici.

```bash
# Lire la documentation Inspektor Gadget
# https://www.inspektor-gadget.io/docs/latest/

# Installer le CLI ig (Inspektor Gadget) — version figée, pas @latest
curl -sL "https://github.com/inspektor-gadget/inspektor-gadget/releases/download/v0.54.1/ig-linux-<ARCH>-v0.54.1.tar.gz" \
  | sudo tar -xzf - -C /usr/local/bin

# Vérifier
ig version

# Installer le plugin kubectl-gadget (nécessaire pour "kubectl gadget ...",
# distinct du binaire ig ci-dessus)
curl -sL "https://github.com/inspektor-gadget/inspektor-gadget/releases/download/v0.54.1/kubectl-gadget-linux-<ARCH>-v0.54.1.tar.gz" \
  | sudo tar -xzf - -C /usr/local/bin

# Déployer Inspektor Gadget sur le cluster kind
kubectl gadget deploy

# PREMIER TEST — tracer les openat du pod nginx
kubectl gadget run trace_open:latest -n default -c nginx-demo
# Dans un autre terminal : kubectl exec nginx-demo -- ls /etc
# Observer les événements qui apparaissent

# Même chose pour les exécutions (execve/execveat) — nécessaire pour
# LANDLOCK_ACCESS_FS_EXECUTE, voir la note sur --paths ci-dessous
kubectl gadget run trace_exec:latest --paths -n default -c nginx-demo
```

**✅ Fait pour `openat` et `execve`** : `internal/tracer.Trace()` n'est
plus un stub — voir `internal/tracer/trace_linux.go`. Il démarre
`trace_open` **et** `trace_exec` concurremment via le SDK Go d'Inspektor
Gadget (runtime gRPC, contre le DaemonSet déjà déployé sur le cluster),
filtre par `opts.Namespace`/`PodName`/`Container`, s'arrête après
`opts.Duration` (`context.WithTimeout`), et fusionne les deux flux en un
seul `[]Event`.

**Pourquoi deux gadgets et pas un seul :** `openat(2)` n'a pas de bit
"exécution" dans ses flags (`O_ACCMODE` ne distingue que
read/write/read_write — contrairement à FreeBSD, Linux n'a pas d'`O_EXEC`).
`trace_open` seul ne peut donc jamais savoir qu'un chemin a été *exécuté* ;
ce signal n'existe que sur `execve(2)`/`execveat(2)`, ce que `trace_exec`
observe directement (avec son paramètre `--paths` activé, pour récupérer
le chemin du binaire exécuté). Ce manque n'a été découvert qu'en testant
en vrai sur le cluster : voir `docs/policy-synthesis.md` pour le détail du
bug (`readExec`/`readWriteExec` n'étaient jamais atteignables avec de
vraies données tant que ce second gadget n'était pas branché).

Point d'architecture important : ce fichier a le build tag `//go:build
linux` — le SDK Inspektor Gadget ne compile pas du tout sur macOS/Windows
(il tire du code Linux-only : eBPF, cgroups...). `tracer.go` (types
`Event`/`Options`, sans le SDK) reste compilable partout ;
`trace_other.go` (`//go:build !linux`) fournit une erreur claire à la
place sur les autres OS. Voir `docs/architecture.md` §3 pour le détail
complet de ce découpage et pourquoi il était nécessaire (pas juste un
choix de style).

**Le réseau (`trace_tcpconnect`/`trace_bind`) n'est délibérément pas
implémenté** : le vrai schéma de la CRD PodLock
(`github.com/flavio/podlock`) n'a aucun champ pour représenter des droits
réseau Landlock — vérifié directement dans son code source, pas supposé.
Voir `docs/roadmap.md` (M1) et `docs/policy-synthesis.md`.

La dépendance est déjà dans `go.mod` (figée à `v0.54.1`, alignée sur les
binaires `ig`/`kubectl-gadget` installés par `hack/init-vm.sh`) :

```bash
grep inspektor-gadget go.mod
```

---

### Étudiant B — `cmd/` + `internal/k8s/` + `internal/policy/`

**Objectif M0 (semaine 1-2) :** avoir une CLI fonctionnelle avec cobra, et
une fonction `Synthesize` testable sur des données mockées.

**Tâche 1 — Remplacer le switch manuel par cobra :**

```bash
go get github.com/spf13/cobra@latest
```

Structure cobra cible pour `cmd/landlock-genprof/main.go` :

```go
var rootCmd = &cobra.Command{Use: "landlock-genprof"}

var traceCmd = &cobra.Command{
    Use:   "trace",
    Short: "Démarre un training run et génère un profil Landlock",
    RunE:  runTrace,
}

func init() {
    traceCmd.Flags().StringP("pod",       "p", "",    "Nom du pod cible")
    traceCmd.Flags().StringP("namespace", "n", "default", "Namespace K8s")
    traceCmd.Flags().DurationP("duration", "d", 60*time.Second, "Durée du training run")
    traceCmd.Flags().StringP("out",       "o", "profile.yaml", "Fichier de sortie")
    traceCmd.MarkFlagRequired("pod")
    rootCmd.AddCommand(traceCmd)
}
```

**Tâche 2 — Implémenter `Synthesize` sur des données mockées :**

Ne pas attendre le tracer. Créer un fichier de test avec des événements statiques :

```go
// internal/policy/synthesize_test.go
func TestSynthesize_AggregatesByDirectory(t *testing.T) {
    events := []tracer.Event{
        {Syscall: "openat", Path: "/usr/share/nginx/index.html", Mode: "read"},
        {Syscall: "openat", Path: "/usr/share/nginx/css/style.css", Mode: "read"},
        {Syscall: "openat", Path: "/tmp/nginx.pid", Mode: "write"},
    }
    rules, err := Synthesize(events)
    // Attendre : /usr/share/nginx → readOnly, /tmp → readWrite
    // Pas de règle par fichier individuel — agrégation par répertoire
}
```

**Tâche 3 — Implémenter `Resolve` dans `k8s/target.go` :**

```bash
go get k8s.io/client-go@latest
```

Utiliser `client-go` pour vérifier que le pod existe avant de démarrer le tracer.

---

### Étudiante C — `docs/threat-model.md` + CI

**Objectif M0 (semaine 1-2) :** compléter le threat model avec les réponses aux
questions ouvertes, et ajouter `gosec` à la CI.

**Tâche 1 — Compléter `docs/threat-model.md` :**

Répondre aux questions ouvertes avec des recherches :

```markdown
## 1. Capacités requises par le tracer

| Capacité     | Pourquoi nécessaire | Alternative moins permissive |
|---|---|---|
| CAP_BPF      | Charger un programme eBPF | ... |
| CAP_SYS_ADMIN| Accès au perf_event_open sur kernels < 5.8 | ... |
```

Sources à consulter :
- [Inspektor Gadget RBAC docs](https://www.inspektor-gadget.io/docs/latest/reference/rbac/)
- [Kubernetes Security Profiles Operator threat model](https://github.com/kubernetes-sigs/security-profiles-operator/blob/main/docs/threat-model.md)

**Tâche 2 — Ajouter `gosec` à la CI :**

Modifier `.github/workflows/ci.yml` :

```yaml
- name: Security scan (gosec)
  uses: securego/gosec@master
  with:
    args: ./...
```

**Tâche 3 — Documenter la méthodologie de validation des profils :**

Répondre dans `docs/threat-model.md` :
- Combien de training runs recommande-t-on avant de faire confiance à un profil ?
- Quels scénarios de test minimaux (démarrage, requête HTTP, erreur 404, reload
  de config) pour couvrir les chemins de code fréquents d'un nginx ?
- Comment détecter qu'un profil `low confidence` a causé une régression en prod ?

---

## 6. Travailler sans dépendre des autres

Le découplage entre les rôles est volontaire. Voici comment avancer sans attendre.

### Étudiant B — sans le tracer d'Étudiant A

Définir une fonction de mock dans les tests :

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

Développer et tester `Synthesize` entièrement avec ces données. L'intégration avec
le vrai tracer se fera en M1 — l'interface `[]Event` est commune.

### Étudiante C — sans le code applicatif

Le threat model et la CI peuvent être développés indépendamment du code Go.
La CI (`go build ./...`, `go vet ./...`) fonctionne déjà sur le scaffolding.
`gosec` peut être ajouté maintenant — il trouvera peu de choses à scanner pour
l'instant, mais la configuration sera en place pour les jalons suivants.

### Étudiant A — sans le reste de la CLI

Le tracer peut être développé et testé en isolation, sans CLI :

```go
// internal/tracer/tracer_test.go (test d'intégration, requiert le cluster)
//go:build integration

func TestTrace_OpenAt(t *testing.T) {
    events, err := Trace(Options{
        PodName:   "nginx-demo",
        Namespace: "default",
        Duration:  10 * time.Second,
    })
    require.NoError(t, err)
    openatEvents := filterBySyscall(events, "openat")
    assert.NotEmpty(t, openatEvents, "aucun openat capturé — le tracer ne fonctionne pas")
}
```

```bash
# Lancer uniquement les tests d'intégration (avec cluster kind actif)
go test -tags integration ./internal/tracer/
```

---

## 7. Lancer la CI en local

Reproduire exactement ce que GitHub Actions va exécuter (sur la VM Linux —
voir la remarque `make docker-test` plus bas pour macOS/Windows) :

```bash
# 1. Vérifier les prérequis kernel
./hack/check-kernel.sh   # ou : make check-kernel

# 2. Build
go build ./...           # ou : make build

# 3. Tests (verbeux + couverture, comme en CI)
go test -v -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
# équivalent (sans le détail par fonction) : make test

# 4. Vet
go vet ./...              # ou : make vet

# 5. SAST — gosec (job "security" séparé en CI, version figée)
go install github.com/securego/gosec/v2/cmd/gosec@v2.28.0
gosec ./...

# 6. SCA — Trivy (dépendances Go / go.sum, nécessite Trivy installé localement)
# https://aquasecurity.github.io/trivy/latest/getting-started/installation/
trivy fs --scanners vuln --severity CRITICAL,HIGH .
```

> 💡 **Sur macOS/Windows**, `make docker-test` fait build + vet + test dans
> un conteneur Linux (`Dockerfile.dev`) — la seule façon d'exercer le vrai
> `internal/tracer/trace_linux.go` (pas le stub) sans la VM. Toujours pas
> de vrai cluster/eBPF dedans (`Dockerfile.dev` s'arrête volontairement à
> build/vet/test), donc ça ne remplace pas `hack/init-vm.sh` pour tester
> `Trace()` en conditions réelles.

**Règle :** la CI doit passer sur `master` à tout moment. Si vous cassez le build,
c'est votre priorité numéro 1 avant toute autre tâche. Le job `security`
(étapes 5-6) ne bloque pas encore les merges (voir `docs/threat-model.md`
§4) mais vaut la peine d'être lancé en local avant de pousser.

---

## 8. Concepts clés à comprendre avant de coder

### Landlock

Landlock est un LSM (_Linux Security Module_) qui permet à un **processus de se
confiner lui-même** sans droits root. Une fois armé, le processus ne peut accéder
qu'aux chemins et ports qu'il a explicitement déclarés.

Lectures indispensables :
- [Landlock — page officielle](https://landlock.io/)
- [`man 7 landlock`](https://man7.org/linux/man-pages/man7/landlock.7.html)
- [Article LWN.net sur Landlock](https://lwn.net/Articles/859908/) — contexte historique

### eBPF et Inspektor Gadget

eBPF permet d'exécuter du code directement dans le kernel Linux, sans modifier
son code source. C'est la technologie utilisée pour observer les syscalls d'un pod
sans l'instrumenter.

**Inspektor Gadget** fournit des gadgets eBPF prêts à l'emploi (pas besoin d'écrire
de l'eBPF from scratch) et un SDK Go pour les consommer.

Lectures :
- [eBPF en 10 minutes](https://ebpf.io/what-is-ebpf/) — introduction accessible
- [Inspektor Gadget quickstart](https://www.inspektor-gadget.io/docs/latest/quick-start/)
- [trace_open gadget](https://www.inspektor-gadget.io/docs/latest/gadgets/trace_open/)

### PodLock et le CRD LandlockProfile

PodLock est un opérateur Kubernetes (écosystème Kubewarden) qui applique des
profils Landlock sur les pods. On génère des fichiers YAML compatibles avec son
CRD `LandlockProfile`.

Lecture :
- [PodLock sur GitHub](https://github.com/flavio/podlock) — lire le README et les
  exemples de CRD

### client-go

`client-go` est la bibliothèque Go officielle pour interagir avec l'API Kubernetes.
Elle est utilisée dans `internal/k8s/target.go` pour vérifier qu'un pod existe
avant de démarrer le tracer.

Lecture :
- [client-go examples](https://github.com/kubernetes/client-go/tree/master/examples)

---

## 9. Questions fréquentes

**Q : Je n'ai pas de machine Linux, comment je fais ?**

Deux options :
- UTM (macOS Apple Silicon) ou VirtualBox (Intel) avec Ubuntu 24.04
- Une VM cloud gratuite (GitHub Codespaces, GCP Free Tier, Oracle Cloud Free Tier)

Le build et les tests unitaires (`go build`, `go test`) fonctionnent sur macOS
ou Windows. Seuls les tests d'intégration (tracer + cluster kind) nécessitent Linux.

---

**Q : `go build ./...` échoue avec des erreurs d'import.**

Normal en M0 : les dépendances réelles ne sont pas encore dans `go.mod`. C'est
la première tâche de M0 — ajouter les `go get` pour Inspektor Gadget, client-go
et sigs.k8s.io/yaml.

---

**Q : Comment savoir si mon commit va casser la CI ?**

Lancer les étapes de la section [7 — Lancer la CI en local](#7-lancer-la-ci-en-local)
avant de pousser.

---

**Q : Étudiant A — le SDK Inspektor Gadget ne fonctionne pas sur mon cluster kind.**

Vérifier que Inspektor Gadget est bien déployé sur le cluster :

```bash
kubectl gadget deploy
kubectl get pods -n gadget
```

Si les pods gadget ne démarrent pas, vérifier les logs :

```bash
kubectl logs -n gadget -l app=gadget
```

La cause la plus fréquente : le kernel hôte ne supporte pas BPF ring buffer
(kernel < 5.8). Sur Ubuntu 24.04, ce n'est pas un problème.

---

**Q : Quelle est la différence entre le plan A (Inspektor Gadget) et le plan B (`strace`) ?**

| | Plan A — Inspektor Gadget | Plan B — strace |
|---|---|---|
| Technologie | eBPF (kernel) | ptrace |
| Overhead | Très faible | Significatif (ptrace bloque à chaque syscall) |
| Prérequis kernel | ≥ 5.8 | Disponible partout |
| Implémentation | SDK Go → API `Trace()` | `strace -f -e trace=openat,...` + parsing |
| Interface `Event{}` | **Identique** | **Identique** |

Si le plan B est activé à la semaine 3-4, seul `internal/tracer/tracer.go` change.
Le reste du pipeline (synthèse, YAML, CLI) ne change pas.

---

**Q : Où poser mes questions ?**

Ouvrir une issue GitHub dans le repo avec le label approprié :
- `question/tracer` — Étudiant A
- `question/policy` — Étudiant B
- `question/threat` — Étudiante C
- `question/setup` — problème d'environnement (tous)
