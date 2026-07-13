# Roadmap

## Décisions d'architecture actées

- Tracer basé sur les gadgets **Inspektor Gadget** existants
  (`trace_open`, `trace_tcpconnect`, ...) plutôt qu'un programme eBPF
  écrit from scratch — réduit fortement le risque d'échec pour une
  équipe démarrant sur eBPF.
- Sortie au format **compatible PodLock** (`LandlockProfile` CRD,
  écosystème Kubewarden) — le projet est complémentaire, pas concurrent.
- Aucune application automatique de policy : revue humaine obligatoire.

## Jalons

- [ ] **M0 — Setup** : repo, licence, CI GitHub Actions
      (`runs-on: ubuntu-24.04` pour garantir un kernel ≥ 6.8),
      script `hack/check-kernel.sh`, cluster `kind` de dev
- [ ] **⚠️ Checkpoint dur — semaine 3-4** : le tracer (Étudiant A) doit
      produire des événements réels pour au moins un type de syscall
      (ex. `openat`), même minimal. **Si ce n'est pas le cas à cette
      date, basculer immédiatement sur le plan de repli** (voir ci-dessous)
      plutôt que d'attendre la fin du semestre.
- [ ] **M1** : tracer fonctionnel sur `openat`/`connect`, CLI `trace`
      opérationnelle en bout en bout sur un pod de test (nginx)
- [ ] **M2** : synthèse de policy (agrégation par répertoire, niveaux de
      confiance), export YAML au format PodLock
- [ ] **M3** : intégration K8s complète (résolution du pod cible, RBAC
      minimal du tracer — voir `docs/threat-model.md`)
- [ ] **M4** : démo e2e sur `kind` — profil généré pour nginx, comparaison
      avec un profil écrit à la main, documentation des écarts
- [ ] **M5 (stretch)** : détection de drift post-déploiement (logs de
      refus Landlock → suggestion d'ajustement de policy)

## Plan de repli si le checkpoint M0→M1 échoue

Si le tracer eBPF (même via Inspektor Gadget) n'est pas fonctionnel au
checkpoint de la semaine 3-4 : basculer la capture d'événements sur
`strace -f` en parsing de sortie. Moins élégant, mais suffisant pour un
training run ponctuel (pas de contrainte de performance de production),
et ça permet aux étudiants B et C de continuer à avancer sans bloquer sur
l'étudiant A.

## Répartition

| Rôle | Composant | Étudiant |
|---|---|---|
| Tracer eBPF | `internal/tracer/` | Étudiant A |
| CLI + intégration K8s | `cmd/`, `internal/k8s/`, `internal/policy/` | Étudiant B |
| Méthodologie / sécurité | `docs/threat-model.md`, tests adversariaux | Étudiante C |
