# landlock-genprof

Générateur automatique de profils de sécurité [Landlock](https://landlock.io/) pour
Kubernetes, par observation (« training run ») plutôt qu'écriture manuelle.

## Le problème

Écrire une policy Landlock à la main nécessite de deviner à l'avance tous les
fichiers, répertoires et ports dont une application aura besoin. En pratique :
- trop permissif → la policy ne protège rien
- trop restrictif → l'application casse en production

Il n'existe pas d'équivalent d'`aa-genprof`/`aa-logprof` (AppArmor) pour
Landlock sur Kubernetes.

## Le principe

1. **Observer** un pod en fonctionnement normal pendant un training run,
   via tracing eBPF (syscalls `openat`, `connect`, `bind`, `execve`, ...)
2. **Synthétiser** les événements observés en un profil Landlock minimal
   (agrégation par répertoire, niveau de confiance par règle)
3. **Générer** un fichier YAML compatible avec le format `LandlockProfile`
   du projet [PodLock](https://github.com/flavio/podlock) (écosystème
   Kubewarden) — `landlock-genprof` est un outil complémentaire, pas un
   remplaçant d'enforcement
4. **Faire réviser** le profil généré par un humain avant tout déploiement
   — jamais d'application automatique

## Statut

🚧 Projet en phase de démarrage (scaffolding initial). Voir `docs/roadmap.md`
et les jalons du board GitHub.

## Structure du repo

```
cmd/landlock-genprof/   CLI (point d'entrée)
internal/tracer/        Capture des événements syscall (via Inspektor Gadget)
internal/policy/        Agrégation des événements → profil LandlockProfile
internal/k8s/           Orchestration d'un training run sur un pod cible
pkg/podlock/            Types Go correspondant au schéma CRD LandlockProfile
examples/                Exemples de training run et de profils générés
docs/                    Méthodologie de validation, threat model, roadmap
hack/                    Scripts de setup (vérification kernel, cluster kind)
```

## Prérequis

- Kernel Linux ≥ 5.13 (Landlock FS), ≥ 6.4 pour le support réseau
- Un cluster de test avec support eBPF (`kind` recommandé, partage le kernel hôte)
- Go 1.22+

Vérifier le support du kernel hôte :

```bash
./hack/check-kernel.sh
```

## Licence

Apache-2.0 — voir [LICENSE](LICENSE)
