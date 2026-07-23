# landlock-genprof

> Version française destinée aux étudiants du projet. La documentation
> principale ([`README.md`](README.md)) et le reste du code sont en anglais —
> voir [`HOW_TO_START.md`](HOW_TO_START.md) pour le guide de prise en main.

Générateur automatique de profils de sécurité [Landlock](https://landlock.io/) pour
Kubernetes, par **observation** d'un pod en fonctionnement (« training run ») plutôt
qu'écriture manuelle des règles.

Le nom est un clin d'œil volontaire à `aa-genprof` / `aa-logprof` — les outils de
génération de profils AppArmor. Landlock n'a pas encore l'équivalent.
`landlock-genprof` comble ce manque.

> **Statut :** scaffolding initial — implémentation en cours avec les étudiants.
> Voir [`docs/roadmap.md`](docs/roadmap.md) pour les jalons et la répartition des tâches.

---

## Sommaire

1. [Le problème](#1-le-problème)
2. [Positionnement — PodLock et l'écosystème Kubewarden](#2-positionnement--podlock-et-lécosystème-kubewarden)
3. [Comment ça marche](#3-comment-ça-marche)
4. [Stack technique](#4-stack-technique)
5. [Architecture du repo](#5-architecture-du-repo)
6. [Prérequis](#6-prérequis)
7. [Démarrage rapide](#7-démarrage-rapide)
8. [Exemple de sortie](#8-exemple-de-sortie)
9. [Équipe et répartition](#9-équipe-et-répartition)
10. [Gestion du risque](#10-gestion-du-risque)
11. [Jalons](#11-jalons)
12. [Threat model](#12-threat-model)
13. [Contribuer](#13-contribuer)
14. [Licence](#14-licence)

---

## 1. Le problème

**Landlock** est un LSM (_Linux Security Module_) apparu dans le kernel 5.13 qui
permet de confiner un processus à un sous-ensemble du système de fichiers et du
réseau, **sans nécessiter de privilèges root**. C'est une propriété rare et
précieuse : là où AppArmor, SELinux ou seccomp nécessitent une configuration
système-wide par l'administrateur, Landlock peut être armé par le processus
lui-même.

### Pourquoi c'est difficile à utiliser en pratique

Écrire une policy Landlock à la main exige de **deviner à l'avance** tous les
chemins, répertoires et ports dont une application aura besoin au cours de sa vie :

- **Trop permissif** → la policy ne protège rien (on autorise tout pour ne rien casser)
- **Trop restrictif** → l'application casse en production sur un chemin de code rare

Le problème est aggravé dans un contexte Kubernetes :

- Landlock n'a **aucune intégration native dans containerd/runc**, donc pas de support
  K8s standard (`securityContext` ne sait pas armer Landlock)
- Il n'existe **aucun équivalent de `aa-genprof`** pour Landlock, ni dans le
  [Security Profiles Operator](https://github.com/kubernetes-sigs/security-profiles-operator)
  ni dans [PodLock](https://github.com/flavio/podlock)

`landlock-genprof` adresse ce vide : observer d'abord, écrire la policy ensuite.

---

## 2. Positionnement — PodLock et l'écosystème Kubewarden

[PodLock](https://github.com/flavio/podlock) (écosystème [Kubewarden](https://www.kubewarden.io/))
est le projet le plus proche. Il fournit :

- Un CRD `LandlockProfile` pour décrire les restrictions d'un pod
- Un opérateur K8s qui applique la policy au démarrage des conteneurs

**Ce que PodLock ne fait pas :** générer les profils. L'utilisateur doit les écrire
à la main, ce qui est précisément le problème adressé ici.

```
                           ┌─────────────────────────────────┐
  landlock-genprof         │  PodLock (Kubewarden)            │
  ──────────────────       │  ─────────────────────────────── │
  observe le pod    ──────►│  LandlockProfile CRD             │
  génère le YAML           │  Opérateur K8s                   │
  (revue humaine)   ──────►│  Enforcement au runtime          │
                           └─────────────────────────────────┘
```

`landlock-genprof` est **complémentaire de PodLock**, pas concurrent. Il génère
des profils dans le format attendu par PodLock, en amont de la chaîne.

---

## 3. Comment ça marche

Le workflow complet se déroule en cinq étapes :

### Étape 1 — Training run

On laisse le pod cible tourner normalement pendant une durée définie (ex. 60 s ou
plus, selon la complexité de l'application). L'objectif est de couvrir les chemins
de code les plus fréquents.

```
landlock-genprof trace \
  --pod nginx-demo \
  --namespace default \
  --binary /usr/sbin/nginx \
  --duration 60s \
  --out profile.yaml
```

### Étape 2 — Capture des syscalls (Tracer)

Pendant le training run, `landlock-genprof` capture les appels système du pod cible
via les **gadgets [Inspektor Gadget](https://www.inspektor-gadget.io/)** :

| Gadget | Syscall observé | Sortie |
|---|---|---|
| `trace_open` | `openat`, `open` | `LANDLOCK_ACCESS_FS_READ_FILE`, `WRITE_FILE`, `EXECUTE` |
| `trace_tcpconnect` | `connect` | `LANDLOCK_ACCESS_NET_CONNECT_TCP` (kernel ≥ 6.4) |
| `trace_bind` | `bind` | `LANDLOCK_ACCESS_NET_BIND_TCP` (kernel ≥ 6.4) |
| `trace_exec` | `execve`, `execveat` | `LANDLOCK_ACCESS_FS_EXECUTE` |
| `advise_seccomp` | tous les syscalls du conteneur | profil seccomp (`--seccomp-out`, voir étape 4) |
| `trace_capabilities` | checks `cap_capable()` | fragment de capacités Linux (`--capabilities-out`, voir étape 4) |

`advise_seccomp` est le gadget "conseiller de profil seccomp" propre à
Inspektor Gadget, réutilisé tel quel plutôt que réimplémenté — il
enregistre déjà les syscalls d'un conteneur et les formate directement
dans le format JSON seccomp cible. Une différence par rapport aux quatre
autres : il observe tous les processus du nœud pendant le run, pas
seulement le conteneur cible (sa propre sonde ne peut pas filtrer plus
tôt sans perdre les syscalls de démarrage du conteneur cible lui-même) —
le filtrage vers le conteneur cible se fait à son propre stade de
formatage. `trace_capabilities` ne partage pas cette particularité : il
filtre au niveau kernel par conteneur de façon classique, comme
`trace_open`/etc.

Chaque événement capturé produit un objet `Event` :

```go
type Event struct {
    Timestamp time.Time
    Syscall   string // "openat", "connect", "bind", "execve", ou un simple nom de syscall/capacité
    Path      string // chemin fichier, si applicable
    Port      int    // port réseau, si applicable
    Mode      string // "read", "write", "read_write", "exec", "egress", "ingress", "syscall", "capability"
}
```

### Étape 3 — Synthèse de policy

Les événements sont agrégés par répertoire (pour éviter le sur-fitting
fichier-par-fichier) et un **niveau de confiance** est calculé pour chaque règle
en fonction de la régularité d'observation sur plusieurs runs :

| Niveau | Signification |
|---|---|
| `high` | Observé systématiquement sur tous les runs — règle fiable |
| `medium` | Observé sur plusieurs runs, mais avec des incohérences |
| `low` | Observé une seule fois — à examiner avant déploiement |

### Étape 4 — Génération du YAML

Le profil est exporté au format `LandlockProfile` CRD de PodLock :

```yaml
apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
  namespace: default
spec:
  profilesByContainer:
    nginx:
      "/usr/sbin/nginx":
        readExec:
          - /lib
          - /lib64
        readOnly:
          - /usr/share/nginx        # confiance: high
        readWrite:
          - /tmp                    # confiance: high
          - /var/cache/nginx/proxy  # confiance: low — à vérifier
```

### Étape 4bis — Génération optionnelle d'une NetworkPolicy

Le CRD de PodLock n'a aucun champ pour les droits réseau : les observations
`connect`/`bind` ont donc leur propre format de sortie. Passer `--network-out`
génère aussi une `NetworkPolicy` Kubernetes à partir du même training run
(ignoré si aucune activité réseau n'a été observée). `--out`/`--network-out`
prennent par défaut un nom dérivé du pod tracé (`<pod>-profile.yaml`,
`<pod>-networkpolicy.yaml`) si passés sans valeur — donne un nom explicite
(`--network-out ma-policy.yaml`) pour le remplacer :

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nginx-demo
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: nginx        # copié depuis les labels du pod tracé
  policyTypes:
    - Egress
  egress:
    - ports:
        - protocol: TCP
          port: 443      # confiance: high
```

Seul le port observé est encodé — aucune restriction `from`/`to` : le
tracer sait qu'un port a été contacté, pas l'identité du pod/service en
face.

### Étape 4ter — Redémarrage optionnel de la cible (`--restart`)

Les ressources qu'un processus ouvre une seule fois au démarrage (un pid
file, un fd de log) puis garde ouvertes sont invisibles à un trace
attaché à un conteneur déjà en cours d'exécution — `trace_open` observe
seulement les `openat()`, pas les `write()` ultérieurs sur un fd déjà
ouvert. Passer `--restart` fait redémarrer la cible par le CLI, en
attachant le tracer *avant* dans tous les cas pour que l'activité de
démarrage soit vraiment capturée — suppression+recréation pour un pod
nu, ou le même mécanisme de rollout restart que `kubectl rollout
restart` pour un pod géré par un Deployment/StatefulSet/DaemonSet.

Le ciblage du tracer diffère selon que le propriétaire garde ou non un
nom de pod stable : un pod nu ou un StatefulSet garde son nom après le
redémarrage, donc le tracer est pré-attaché directement sur ce nom. Le
remplacement d'un Deployment/DaemonSet obtient un nom imprévisible, donc
le tracer est plutôt pré-attaché via le **label selector du workload**
lui-même — ce qui signifie aussi que le profil généré est identifié par
le nom du *workload* (ex. `nginx-ds`), pas par un pod éphémère, et que le
rappel PodLock patche le template de pods (`kubectl patch deployment`/
`daemonset`) plutôt que de labelliser un seul pod qu'un futur rollout
remplacerait de toute façon.

Opt-in : c'est perturbateur pour la charge de travail en cours, et ça
nécessite des RBAC supplémentaires au-delà du manifeste de base —
applique [`deploy/rbac-restart.yaml`](deploy/rbac-restart.yaml) d'abord.

### Étape 4quater — Historique multi-run optionnel (`--history`)

`Confidence` est censé refléter combien de training runs séparés ont
observé un accès ("vu à chaque run" vs "vu une fois sur 5"), mais un seul
run de `trace` n'a aucun moyen de le savoir — il ne peut mesurer que le
nombre de fois où quelque chose a été vu *dans ce run*. Passer
`--history` persiste une custom resource `TrainingHistory`
(`internal/history`, sans controller — le CLI la lit/écrit directement)
qui accumule à travers chaque run `--history` pour le même
container/binaire, pour que `Confidence` puisse enfin être calculée à
partir du vrai ratio. Nécessite le CRD et des RBAC supplémentaires,
appliqués une fois :
[`deploy/crd-traininghistory.yaml`](deploy/crd-traininghistory.yaml),
[`deploy/rbac-history.yaml`](deploy/rbac-history.yaml). Consulte le
résultat directement : `kubectl get traininghistory
<container>-<basename-du-binaire> -o yaml`. `profile.yaml`/
`networkpolicy.yaml`/`capabilities.yaml` l'affichent aussi désormais —
chaque chemin/port/capacité a un commentaire `# confidence: ...` en fin
de ligne (voir Étape 4), et avec `--history` ce commentaire reflète le
vrai ratio multi-run au lieu de l'estimation single-run utilisée sans ce
flag. `seccomp.json` (étape 4quinquies) ne peut pas porter de
commentaire — sa confiance est affichée sur stdout à la place.

### Étape 4quinquies — Génération optionnelle d'un profil seccomp (`--seccomp-out`)

Passer `--seccomp-out` génère aussi un profil seccomp à partir du même
training run (ignoré si aucun syscall n'a été observé), via le gadget
`advise_seccomp` d'Inspektor Gadget (voir le tableau des gadgets à
l'étape 2) :

```json
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": ["SCMP_ARCH_X86_64"],
  "syscalls": [
    {
      "names": ["accept4", "epoll_wait", "openat", "read", "write"],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
```

Volontairement en JSON pur, pas en YAML avec un commentaire `#
confidence: ...` comme les deux autres sorties : ce fichier est chargé
directement par le kubelet/le runtime de conteneur (référencé via
`securityContext.seccompProfile.localhostProfile` d'un pod, jamais
`kubectl apply`é), donc il doit rester du JSON valide, sans commentaire.
À la place, le CLI affiche sur stdout les syscalls non encore confirmés
sur plusieurs runs `--history` après avoir écrit le fichier — sur un
seul run sans `--history`, cela veut dire tous les syscalls, puisque
`advise_seccomp` rapporte un ensemble dédupliqué par run plutôt qu'un
comptage par occurrence, donc `Confidence` ne peut être que `low` tant
que `--history` n'a pas accumulé plusieurs runs.

Celui-ci mérite d'être pris au sérieux avant d'être appliqué : un
syscall manquant ne restreint pas juste l'accès comme le ferait une
`NetworkPolicy` trop stricte — il casse le conteneur purement et
simplement. Préfère `--history` à un seul run avant de le déployer.

### Étape 4sexies — Fragment de capacités Linux optionnel (`--capabilities-out`)

Passer `--capabilities-out` génère aussi un fragment de capacités Linux à
partir du même training run (ignoré si aucun check de capacité n'a été
observé), via le gadget `trace_capabilities` d'Inspektor Gadget (voir le
tableau des gadgets à l'étape 2) :

```yaml
add:
  - NET_BIND_SERVICE   # confidence: high
drop:
  - ALL
```

Contrairement aux trois autres sorties, ce n'est pas un artefact complet
et autonome : les capacités Linux ne vivent jamais que dans le champ
`securityContext.capabilities` d'un conteneur, il n'existe pas
d'équivalent d'une `NetworkPolicy` ou d'un profil seccomp à générer seul.
Ce fichier est un simple fragment à coller directement sous cette clé —
`drop: [ALL]` toujours, `add` listant chaque capacité observée
(convention Kubernetes de nom court, préfixe `CAP_` retiré). Comme ce
fragment est destiné à être collé manuellement, pas chargé directement
par le kubelet, il garde le même style de commentaire `# confidence:
...` que `profile.yaml`/`networkpolicy.yaml`.

### Étape 4septies — securityContext composé optionnel (`--security-context-out`)

Passer `--security-context-out` génère aussi un fragment `securityContext`
composé, combinant les mêmes données de capacités que l'étape 4sexies
avec une *référence* au profil seccomp de l'étape 4quinquies (seulement
si `--seccomp-out` a aussi été passé et a réellement produit un fichier
ce run-ci — jamais une référence vers un fichier qui n'existe pas) :

```yaml
capabilities:
  add:
    - NET_BIND_SERVICE   # confidence: high
  drop:
    - ALL
seccompProfile:
  type: Localhost
  localhostProfile: nginx-demo-seccomp.json
```

Ce n'est **pas** une fusion des exporteurs seccomp et capabilities —
`seccomp.json`/`capabilities.yaml` sont toujours générés exactement
comme avant, indépendamment. Un profil seccomp doit être livré comme son
propre fichier pour que le kubelet le charge (`localhostProfile` ne
prend jamais qu'une référence de chemin, jamais de contenu inline), donc
fusionner les fichiers eux-mêmes ne réduirait rien du tout — ça
ajouterait juste de l'indirection. Ce flag ajoute une troisième vue,
composée, en plus, pour le cas courant où on veut les deux au même
endroit à coller sous la clé `securityContext:` d'un conteneur.
`localhostProfile` n'est jamais que le nom de base du fichier seccomp —
copie ce fichier exact vers `/var/lib/kubelet/seccomp/` sur chaque nœud
sous ce même nom pour que la référence soit valide.

**Volontairement, ceci n'infère pas** `privileged`,
`allowPrivilegeEscalation`, `runAsNonRoot`, `readOnlyRootFilesystem`, ni
`runAsUser` — rien dans ce projet n'observe aucun de ces champs
aujourd'hui, et deviner des "valeurs par défaut sûres" indépendamment de
ce qui a réellement été observé contredirait le positionnement même du
projet : observer, pas deviner.

### Étape 5 — Revue humaine obligatoire

**`landlock-genprof` ne déploie jamais un profil automatiquement.**
Le YAML généré est un point de départ pour la revue humaine, pas un résultat final.
Le champ `Confidence` par règle rend explicite ce qui est sûr et ce qui demande
attention. Voir [`docs/threat-model.md`](docs/threat-model.md) pour la méthodologie
de validation recommandée.

**Appliquer un `LandlockProfile` seul n'a aucun effet.** Le webhook
d'admission de PodLock associe un pod en cours d'exécution à un objet
`LandlockProfile` via un label sur le *pod* —
`podlock.kubewarden.io/profile: <nom-du-profil>` — pas via quoi que ce
soit d'intégré au CRD lui-même. `landlock-genprof trace` affiche la
commande `kubectl label` exacte à lancer après le `kubectl apply` du
profil généré.

---

## 4. Stack technique

| Composant | Choix | Justification |
|---|---|---|
| Langage | **Go 1.26** | Écosystème K8s natif (client-go, controller-runtime) ; SDK Inspektor Gadget en Go |
| Tracer | **[Inspektor Gadget](https://www.inspektor-gadget.io/)** | Gadgets eBPF déjà écrits et testés par la communauté CNCF — évite d'écrire de l'eBPF from scratch (risque élevé pour des débutants) |
| Format de sortie | **LandlockProfile CRD** ([PodLock](https://github.com/flavio/podlock)) | Format existant, écosystème Kubewarden — complémentaire, pas concurrent |
| Cluster de dev | **[kind](https://kind.sigs.k8s.io/)** | Partage le kernel hôte — requis pour que Landlock et eBPF fonctionnent |
| CI | **GitHub Actions** (`ubuntu-24.04`) | Kernel 6.8 — couvre FS + réseau Landlock |
| Licence | **Apache-2.0 OR MIT** | Double licence au choix (convention `landlock-lsm/island`) — compatible avec PodLock et l'écosystème CNCF |

**Principales dépendances Go** (toutes figées à une version exacte dans
`go.mod`, jamais `@latest`) :

```
github.com/inspektor-gadget/inspektor-gadget  # SDK tracer (Linux uniquement, voir internal/tracer)
sigs.k8s.io/yaml                               # sérialisation YAML
k8s.io/client-go                               # résolution pod cible
github.com/spf13/cobra                         # CLI
```

---

## 5. Architecture du repo

> Diagrammes de flux (composants, séquence d'un training run, dépendances
> entre packages) : voir [`docs/architecture.md`](docs/architecture.md).

```
landlock-genprof/
│
├── cmd/landlock-genprof/      Point d'entrée CLI
│   └── main.go                Dispatch des sous-commandes (trace, version)
│
├── internal/
│   ├── tracer/                Capture des événements syscall
│   │   └── tracer.go          Types Event, Options — intégration Inspektor Gadget
│   ├── policy/                Agrégation événements → IR de comportement
│   │   └── synthesize.go      Synthesize() — algorithme d'agrégation (indépendant du format de sortie)
│   ├── profile/                IR de comportement — indépendant de tout format de sortie
│   │   └── profile.go         BehaviorProfile, FilesystemProfile, FileAccess, Confidence
│   ├── exporter/
│   │   ├── podlock/           Conversion IR → PodLock (seul package dépendant des deux)
│   │   │   └── export.go      ToProfile(), ToYAML()
│   │   ├── networkpolicy/     Conversion IR → NetworkPolicy Kubernetes
│   │   │   └── export.go      ToPolicy(), ToYAML()
│   │   ├── seccomp/           Conversion IR → profil seccomp
│   │   │   └── export.go      ToProfile(), ToJSON()
│   │   ├── capabilities/      Conversion IR → fragment de capacités Linux
│   │   │   └── export.go      ToProfile(), ToYAML()
│   │   └── securitycontext/   Compose capacités + référence seccomp
│   │       └── export.go      ToSecurityContext(), ToYAML()
│   ├── history/                CRD TrainingHistory (Confidence multi-run)
│   │   └── record.go          Record, Merge(), ApplyConfidence()
│   └── k8s/                   Orchestration du pod cible
│       └── target.go          Résolution namespace/pod/container via client-go
│
├── pkg/
│   ├── podlock/                Types Go du CRD LandlockProfile (PodLock)
│   │   └── types.go           LandlockProfile, Profile, Metadata
│   └── seccomp/                Types Go du format JSON de profil seccomp
│       └── types.go           Profile, SyscallRule
│
├── examples/
│   └── nginx-generated-profile.yaml   Exemple illustratif de profil généré
│
├── docs/
│   ├── roadmap.md             Jalons, répartition, plan de repli
│   └── threat-model.md        Surface d'attaque, méthodologie de validation
│
├── hack/
│   └── check-kernel.sh        Vérification prérequis kernel (Landlock + eBPF)
│
├── .github/workflows/
│   └── ci.yml                 Build, test, vet (ubuntu-24.04 / kernel 6.8)
│
├── Makefile                   Targets build/test/vet/docker-* (voir `make help`)
├── Dockerfile.dev             Build/test dans un conteneur Linux sans la VM
├── go.mod
├── LICENSE-APACHE             Texte complet Apache-2.0
├── LICENSE-MIT                Texte complet MIT
├── COPYRIGHT                  Explique le choix "au choix" entre les deux
├── README.md                  Documentation principale (anglais)
└── README.etudiants.md        Ce document (français)
```

---

## 6. Prérequis

### Kernel Linux

La seule vraie contrainte de landlock-genprof, c'est la **version du
kernel** — pas une distro en particulier. Rien dans `hack/` n'appelle un
gestionnaire de paquets spécifique (`apt`/`dnf`/`yum`, ...) :
`check-kernel.sh`/`init-vm.sh` n'utilisent que `uname`, `curl`, `tar`, et
des outils Linux génériques. Toute distro avec un kernel assez récent
devrait fonctionner.

| Fonctionnalité | Version minimale du kernel | Notes |
|---|---|---|
| Landlock FS | **≥ 5.13** | Confinement fichiers/répertoires |
| Landlock réseau | **≥ 6.4** | Confinement TCP (connect/bind) |
| eBPF (Inspektor Gadget) | **≥ 5.8** recommandé | BPF ring buffer |

**Testé en vrai** (liste de ce qui est confirmé, pas une restriction —
voir ci-dessus) :

| Distro | Kernel | Statut |
|---|---|---|
| Ubuntu 24.04 | 6.8 | ✅ validé |
| Ubuntu 26.04 | 7.0 | ✅ validé |

Vérification des prérequis de la machine hôte :

```bash
./hack/check-kernel.sh
```

### Outils

```bash
go 1.26+        # Build et tests
kind            # Cluster K8s local (partage le kernel hôte)
kubectl         # Interaction cluster
```

### Installation de kind et création du cluster de dev

```bash
# Installer kind (version figée, pas @latest)
go install sigs.k8s.io/kind@v0.32.0

# Créer le cluster
kind create cluster --name landlock-dev

# Vérifier
kubectl cluster-info --context kind-landlock-dev
```

> `./hack/init-vm.sh` (ou `make init-vm`) automatise ceci en plus de
> kubectl, Inspektor Gadget et d'un pod de test, en une seule commande
> idempotente — voir `HOW_TO_START.md` §2 pour le détail de ce qu'il fait.

---

## 7. Démarrage rapide

```bash
# Cloner le repo
git clone git@github.com:idriss-eliguene/landlock-genprof.git
cd landlock-genprof

# Vérifier les prérequis kernel
./hack/check-kernel.sh

# Build
go build ./...

# Tests (unitaires — pas de cluster requis)
go test ./...

# CLI (pipeline complet, y compris internal/tracer)
go run ./cmd/landlock-genprof trace --pod nginx --namespace default --binary /usr/sbin/nginx --duration 60s --out profile.yaml
```

### Installation en tant que plugin kubectl

`landlock-genprof` fonctionne en standalone (ci-dessus), mais s'installe
aussi comme plugin `kubectl` : un plugin n'est rien de plus qu'un
exécutable nommé `kubectl-<nom>` quelque part dans le `PATH` — `kubectl
<nom>` le trouve et l'exécute. L'outil résout déjà le kubeconfig de la
même façon que `kubectl` lui-même (`internal/k8s.RestConfig()`), donc
aucun changement de code n'a été nécessaire, juste un build sous un autre
nom :

```bash
make install-plugin   # build kubectl-landlock-genprof et l'installe dans $(go env GOPATH)/bin
kubectl plugin list   # confirme que kubectl le voit
kubectl landlock-genprof trace --pod nginx --namespace default --binary /usr/sbin/nginx --duration 60s
```

Une particularité des plugins kubectl à connaître : les flags globaux
`kubectl` placés *avant* le nom du plugin (`kubectl -n foo
landlock-genprof ...`) ne sont **pas** transmis au plugin — kubectl ne
passe que les arguments placés *après* le nom du plugin. Utilise plutôt
le flag `-n`/`--namespace` propre à `landlock-genprof` :
`kubectl landlock-genprof trace -n foo ...`.

---

## 8. Exemple de sortie

Profil généré pour un pod nginx après un training run de 60 s.
Voir [`examples/nginx-generated-profile.yaml`](examples/nginx-generated-profile.yaml).

```yaml
apiVersion: podlock.kubewarden.io/v1alpha1
kind: LandlockProfile
metadata:
  name: nginx-demo
  namespace: default
spec:
  profilesByContainer:
    nginx:
      "/usr/sbin/nginx":
        readExec:
          - /lib
          - /lib64
        readOnly:
          - /usr/share/nginx        # confiance: high — vu à chaque run
        readWrite:
          - /tmp                    # confiance: high — vu à chaque run
          - /var/cache/nginx/proxy  # confiance: low — vu 1 fois sur 5 runs
```

Le champ `confiance` rend **explicite** ce qui est fiable et ce qui nécessite
une vérification avant déploiement en production.

---

## 9. Équipe et répartition

Projet réalisé à 3 étudiants. Chaque rôle est indépendant pour permettre un
avancement en parallèle dès le départ.

| Étudiant | Composant | Focus technique |
|---|---|---|
| **Étudiant A** | `internal/tracer/` | Intégration SDK Inspektor Gadget, mapping syscalls → droits Landlock, formats d'événements |
| **Étudiant B** | `cmd/`, `internal/k8s/`, `internal/policy/` | CLI (cobra), orchestration K8s via client-go, algorithme de synthèse et agrégation par répertoire |
| **Étudiante C** | `docs/threat-model.md`, tests adversariaux, CI | Méthodologie de validation des profils, surface d'attaque du tracer, pentest (évasion, RBAC), durcissement CI (gosec, Trivy) |

### Comment travailler en parallèle dès la semaine 1

Les étudiants B et C **n'ont pas besoin que le tracer soit fonctionnel** pour
avancer. Des données de trace mockées (un `[]Event` statique codé en dur dans les
tests) permettent de développer la synthèse de policy et le threat model
indépendamment. L'intégration réelle avec le tracer d'Étudiant A se fait en M1.

---

## 10. Gestion du risque

### Risque principal : l'eBPF est difficile pour des débutants

L'eBPF est réputé complexe (vérificateur kernel, CO-RE, bpftool). Deux mitigations
ont été actées dès la conception :

**Mitigation 1 — Ne pas écrire de l'eBPF from scratch**

On consomme les gadgets **Inspektor Gadget** existants via leur SDK Go
(`trace_open`, `trace_tcpconnect`, etc.). Ces gadgets sont écrits, testés et
maintenus par la communauté CNCF. L'étudiant A n'écrit pas de programme eBPF —
il appelle une API Go qui retourne des `Event`.

**Mitigation 2 — Checkpoint dur à la semaine 3-4**

Si le tracer ne produit pas d'événements réels (`openat` au minimum) à la semaine
3-4, **bascule immédiate sur le plan de repli** : capturer les événements via
`strace -f` et parser la sortie. Moins élégant qu'eBPF, mais :

- Suffisant pour un training run ponctuel (pas de contrainte de performance)
- Les étudiants B et C ne sont pas bloqués
- Le reste du pipeline (synthèse, génération YAML, CLI) reste identique

```
Plan A (nominal)      Plan B (repli semaine 3-4)
─────────────────     ──────────────────────────
Inspektor Gadget  →   strace -f + parsing
  SDK Go              (même interface Event{})
  eBPF kernel         pas de prérequis kernel eBPF
```

### Risque secondaire : complétude des profils générés

Un training run court ne couvre pas tous les chemins de code (erreurs, cas limites,
comportements déclenchés rarement). Un profil incomplet peut casser l'application
en production sur un chemin non observé. Mitigation : le champ `Confidence` par
règle rend ce risque **visible** dans le YAML plutôt que de donner une fausse
impression de complétude. Voir [`docs/threat-model.md`](docs/threat-model.md).

---

## 11. Jalons

| Jalon | Contenu | Responsable |
|---|---|---|
| **M0 — Setup** | Repo, CI, `go.mod` avec dépendances réelles, `hack/check-kernel.sh`, cluster kind | Tous |
| ⚠️ **Checkpoint semaine 3-4** | Le tracer produit des événements réels sur au moins `openat`. Sinon : bascule sur `strace` | Étudiant A |
| **M1** | Tracer fonctionnel (`openat` + `connect`), CLI `trace` bout en bout sur un pod nginx | A + B |
| **M2** | Synthèse de policy (agrégation par répertoire, niveaux de confiance), export YAML PodLock | B + C |
| **M3** | Intégration K8s complète (résolution pod via client-go, RBAC minimal du tracer) | B + C |
| **M4** | Démo e2e sur kind — profil généré pour nginx, comparaison avec profil écrit à la main | Tous |
| **M5 _(stretch)_** | Détection de drift post-déploiement : logs de refus Landlock → suggestion d'ajustement | Tous |

---

## 12. Threat model

Le tracer lui-même introduit une surface d'attaque : il nécessite des capacités
élevées (`CAP_BPF`, `CAP_SYS_ADMIN` selon le kernel) pour observer les syscalls
d'un pod. Questions ouvertes à documenter dans [`docs/threat-model.md`](docs/threat-model.md) :

- Quelles capacités précises sont nécessaires, et peut-on les réduire ?
- Le tracer doit-il tourner en permanence ou uniquement pendant le training run ?
- Quel RBAC minimal pour le service account du tracer (namespace dédié, pas de
  droits cluster-wide au-delà du strict nécessaire) ?
- Un pod observé peut-il **détecter qu'il est tracé** et modifier son comportement
  pour générer un profil artificiel (évasion) ?
- Le workflow de revue humaine peut-il être court-circuité en pratique ?

---

## 13. Contribuer

Ce projet est un projet pédagogique. Les contributions externes sont bienvenues
après la fin du semestre. Pour l'instant, le développement actif se fait dans les
branches des étudiants :

```
master      → scaffolding stable, decisions d'architecture
feat/tracer → Étudiant A (internal/tracer)
feat/policy → Étudiant B (internal/policy + k8s + cmd)
feat/threat → Étudiante C (docs + CI)
```

---

## 14. Licence

Double licence, au choix de qui réutilise ce code : [Apache-2.0](LICENSE-APACHE)
**ou** [MIT](LICENSE-MIT) — voir [`COPYRIGHT`](COPYRIGHT). Convention reprise de
[`landlock-lsm/island`](https://github.com/landlock-lsm/island), l'outil de
sandboxing Landlock officiel. Compatible avec PodLock et l'écosystème CNCF.
