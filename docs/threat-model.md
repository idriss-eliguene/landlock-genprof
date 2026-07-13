# Threat model & méthodologie de validation

> Document vivant, à compléter au fil de l'avancement du projet.
> Propriétaire : Étudiante C (volet cybersécurité).

## 1. Surface d'attaque du tracer

Le tracer a besoin de privilèges élevés (capacités eBPF) pour observer les
syscalls d'un pod cible. C'est en soi une nouvelle surface d'attaque
introduite dans le cluster.

Questions à documenter :
- Quelles capacités précises sont nécessaires (`CAP_BPF`, `CAP_SYS_ADMIN`
  selon la version kernel) ?
- Le tracer doit-il tourner en permanence ou uniquement pendant le
  training run (préférable) ?
- Quel RBAC minimal pour le service account du tracer ?
- Quel blast radius si le tracer lui-même est compromis ?

## 2. Complétude des profils générés (risque de faux négatif)

Un training run court ne couvre pas tous les chemins de code possibles
(erreurs, cas limites, comportements déclenchés rarement). Un profil
généré par observation peut donc être :
- trop restrictif s'il manque des règles légitimes (l'app casse en prod)
- silencieusement incomplet si personne ne sait ce qui n'a pas été observé

Méthodologie à définir :
- Comment mesurer/exprimer la couverture d'un training run ?
- Comment le YAML généré doit-il communiquer le niveau de confiance par
  règle (voir `internal/policy.Confidence`) plutôt que de donner une
  fausse impression de complétude ?
- Protocole recommandé : combien de runs, sur quelle durée, avec quels
  scénarios de test (y compris chemins d'erreur) ?

## 3. Pentest de l'opérateur / du profil généré

Une fois un profil déployé (via PodLock), tenter de le contourner :
- Un pod peut-il échapper à sa policy Landlock ?
- Le processus tracé peut-il détecter qu'il est observé et modifier son
  comportement pendant le training run (évasion) ?
- Le workflow de revue humaine peut-il être court-circuité en pratique ?

## 4. Durcissement CI

- Intégration d'un scan SAST/SCA (ex. `gosec`, Trivy) sur le code Go du
  projet, dans `.github/workflows/ci.yml`.

---

*Ce fichier sert de point de départ. Le rapport final (format proche
STRIDE ou équivalent) sera construit au fur et à mesure que l'architecture
se stabilise.*
