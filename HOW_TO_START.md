# Comment démarrer — Guide d'onboarding étudiant

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

Vérifie que le kernel supporte Landlock :

```bash
# Depuis le repo cloné (section 2) :
./hack/check-kernel.sh
```

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

### Étape 1 — Vérifier le kernel

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

### Étape 2 — Installer Go

```bash
# Vérifier la version installée
go version   # doit afficher go1.22 ou supérieur

# Si absent, installer depuis https://go.dev/dl/
# Sur Ubuntu :
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### Étape 3 — Cloner le repo

```bash
git clone git@github.com:idriss-eliguene/landlock-genprof.git
cd landlock-genprof
```

### Étape 4 — Builder et tester

```bash
# Build — doit passer sans erreur
go build ./...

# Tests unitaires (pas de cluster requis)
go test ./...

# Vérification statique
go vet ./...
```

> Si `go build` échoue avec des erreurs d'import, c'est normal tant que les
> dépendances réelles ne sont pas encore ajoutées dans `go.mod` (tâche M0).

### Étape 5 — Installer kind (cluster Kubernetes local)

kind (_Kubernetes IN Docker_) crée un cluster K8s local en utilisant Docker.
Il partage le kernel hôte, ce qui est indispensable pour que Landlock et eBPF
fonctionnent.

```bash
# Installer kind
go install sigs.k8s.io/kind@latest

# Installer kubectl (si absent)
# https://kubernetes.io/docs/tasks/tools/

# Créer le cluster de dev
kind create cluster --name landlock-dev

# Vérifier
kubectl cluster-info --context kind-landlock-dev
kubectl get nodes
```

Sortie attendue :

```
NAME                        STATUS   ROLES           AGE
landlock-dev-control-plane  Ready    control-plane   30s
```

### Étape 6 — Déployer un pod de test (nginx)

```bash
kubectl run nginx-demo --image=nginx:alpine --port=80
kubectl wait --for=condition=Ready pod/nginx-demo --timeout=60s
kubectl get pod nginx-demo
```

Ce pod sera la cible des premiers tests du tracer.

---

## 3. Explorer le code existant

Avant d'écrire la moindre ligne, lire ces fichiers dans l'ordre :

```
1. README.md                         → vision globale du projet
2. docs/roadmap.md                   → jalons et répartition
3. docs/threat-model.md              → surface d'attaque (Étudiante C)
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

Sortie de la dernière commande — ce sont les tâches à implémenter :

```
internal/k8s/target.go:      panic("not implemented")   ← M1, Étudiant B
internal/policy/synthesize.go: panic("not implemented") ← M2, Étudiant B
internal/tracer/tracer.go:   panic("not implemented")   ← M1, Étudiant A
cmd/landlock-genprof/main.go: // TODO(M1): brancher ... ← M1, Étudiant B
```

---

## 4. Workflow Git

### Branches

```
main          → code stable, toujours buildable et testable
feat/tracer   → Étudiant A (internal/tracer/)
feat/policy   → Étudiant B (internal/policy/ + internal/k8s/ + cmd/)
feat/threat   → Étudiante C (docs/ + CI)
```

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
# 1. Récupérer les dernières modifications de main
git fetch origin
git rebase origin/main

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

**Ne jamais pousser directement sur `main`.** Toujours passer par une Pull Request
— même entre étudiants, même pour une petite modification. Ça permet à l'enseignant
de suivre l'avancement et à l'équipe de se relire.

---

## 5. Premières tâches par rôle

### Étudiant A — `internal/tracer/`

**Objectif M0 (semaine 1-2) :** comprendre Inspektor Gadget et faire tourner
un gadget existant sur le cluster kind.

```bash
# Lire la documentation Inspektor Gadget
# https://www.inspektor-gadget.io/docs/latest/

# Installer le CLI ig (Inspektor Gadget)
IG_VERSION=$(curl -s https://api.github.com/repos/inspektor-gadget/inspektor-gadget/releases/latest \
  | grep '"tag_name"' | cut -d '"' -f 4)
curl -sL "https://github.com/inspektor-gadget/inspektor-gadget/releases/download/${IG_VERSION}/ig-linux-amd64.tar.gz" \
  | sudo tar -xzf - -C /usr/local/bin

# Vérifier
ig version

# Déployer Inspektor Gadget sur le cluster kind
kubectl gadget deploy

# PREMIER TEST — tracer les openat du pod nginx
ig trace open --containername nginx-demo
# Dans un autre terminal : kubectl exec nginx-demo -- ls /etc
# Observer les événements qui apparaissent
```

**Tâche concrète :** remplacer `panic("not implemented")` dans `tracer.go` par
une implémentation qui :
1. Démarre un gadget `trace_open` via le SDK Go d'Inspektor Gadget
2. Filtre les événements pour le pod cible (`opts.PodName`)
3. Arrête la capture après `opts.Duration`
4. Retourne une `[]Event`

**Dépendance à ajouter dans `go.mod` :**

```bash
go get github.com/inspektor-gadget/inspektor-gadget@latest
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

Reproduire exactement ce que GitHub Actions va exécuter :

```bash
# 1. Vérifier les prérequis kernel
./hack/check-kernel.sh

# 2. Build
go build ./...

# 3. Tests
go test ./...

# 4. Vet
go vet ./...

# 5. (M0 — à ajouter) gosec
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...
```

**Règle :** la CI doit passer sur `main` à tout moment. Si vous cassez le build,
c'est votre priorité numéro 1 avant toute autre tâche.

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
