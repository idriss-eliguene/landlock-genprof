# Synthèse de policy — `internal/policy.Synthesize`

Ce document explique les décisions de conception derrière `Synthesize()`
(jalon M2). Le code lui-même documente le *quoi* en commentaires ; ce
fichier documente le *pourquoi*, pour qui doit modifier l'algorithme sans
relire tout l'historique de conception.

## Le problème

Entrée : un `[]tracer.Event` brut (un accès syscall = un event, potentiellement
des centaines par training run). Sortie : un `[]Rule`, une par répertoire,
avec une catégorie d'accès et un niveau de confiance — au format consommable
par `pkg/podlock.BinaryProfile`.

Le risque à éviter : une règle par fichier individuel. Ça produirait des
profils illisibles (des centaines d'entrées) et sur-fittés au training run
exact plutôt que généralisables à l'usage normal de l'application.

## Agrégation par répertoire, plafonnée à 3 segments

`aggregationDir()` ne prend pas juste `filepath.Dir(path)` — elle tronque le
résultat à `maxAggregationDepth = 3` segments depuis la racine. Sans cette
troncature, deux fichiers dans des sous-répertoires différents d'un même
projet produiraient deux règles distinctes :

```
/usr/share/nginx/html/index.html   → filepath.Dir seul : /usr/share/nginx/html
/usr/share/nginx/css/style.css     → filepath.Dir seul : /usr/share/nginx/css
```

alors que l'exemple de référence (README §8) attend une seule règle
`/usr/share/nginx` pour les deux. La profondeur 3 est un choix empirique
calé sur cet exemple — pas une constante dérivée d'une propriété du système
de fichiers. Si un jour les profils générés s'avèrent trop larges (englobent
des sous-répertoires qui devraient être distingués) ou trop fins, c'est ce
paramètre qu'il faut revoir en premier, pas l'algorithme autour.

## Catégorisation : pourquoi `write` prime sur `read`

```go
switch {
case acc.write:
    access = append(access, "readWrite")
case acc.read:
    access = append(access, "readOnly")
}
```

`readWrite` est traité comme un sur-ensemble de `readOnly`, pas une
catégorie séparée à cumuler. Un répertoire où on a vu au moins une écriture
est classé entièrement `readWrite`, jamais `readOnly` + `readWrite` en même
temps. `readExec` en revanche est indépendant et peut se cumuler avec l'une
ou l'autre — un répertoire peut légitimement contenir un binaire exécuté et
un fichier de config lu.

## Pourquoi les events réseau (`connect`/`bind`) sont ignorés

`Synthesize` filtre tout event sans `Path` (`ev.Path == ""`). Ce n'est pas
un oubli : `pkg/podlock.BinaryProfile` (voir `pkg/podlock/types.go`) n'a que
`ReadExec`/`ReadOnly`/`ReadWrite` — aucun champ pour représenter des droits
réseau Landlock (`LANDLOCK_ACCESS_NET_BIND_TCP` /
`LANDLOCK_ACCESS_NET_CONNECT_TCP`). Générer une `Rule` pour un event réseau
produirait une donnée qu'on ne pourrait jamais sérialiser en sortie. Tant
que le schéma PodLock ne couvre pas le réseau, ces events n'ont nulle part
où atterrir.

**Limitation connue :** si `pkg/podlock.BinaryProfile` gagne un jour un
champ réseau, ce filtre devra être retiré et une agrégation équivalente
(par port ? par plage ?) devra être ajoutée à `dirAccess`.

## Confidence : une heuristique volontairement provisoire

```go
func confidenceFor(seenCount int) Confidence {
    switch {
    case seenCount >= 3: return ConfidenceHigh
    case seenCount == 2: return ConfidenceMedium
    default:             return ConfidenceLow
    }
}
```

La définition officielle de `Confidence` (voir le commentaire du type, et
`docs/threat-model.md` §2) est "vu sur combien de **training runs**
distincts" — l'exemple du README dit littéralement *"vu à chaque run"* vs
*"vu 1 fois sur 5 runs"*. Ce que `confidenceFor` calcule aujourd'hui est
différent : le nombre d'événements agrégés dans **un seul** appel de
`Synthesize`, donc dans **un seul** run.

C'est un proxy raisonnable (un répertoire touché 3 fois dans un run a
statistiquement plus de chances d'être un chemin stable), mais **ce n'est
pas la vraie mesure**. La vraie mesure demande de faire persister l'état
entre plusieurs appels de `Synthesize` (un par run), ce qui n'est pas câblé
— voir roadmap M5. Ne pas présenter les valeurs de `Confidence` actuelles
comme fiables au sens du threat model tant que cette limitation n'est pas
levée.

## Déterminisme

Les clés de `map[string]*dirAccess` sont triées (`sort.Strings`) avant de
construire le `[]Rule` final. Sans ce tri, l'ordre d'itération d'une map Go
n'est pas garanti stable d'un run à l'autre — deux appels de `Synthesize`
sur les mêmes données pourraient produire un `[]Rule` dans un ordre
différent, cassant des tests et rendant les diffs de YAML générés
illisibles en revue.

## Voir aussi

- `internal/policy/synthesize.go` — l'implémentation
- `internal/policy/synthesize_test.go` — cas de test (agrégation par
  répertoire, events mockés nginx, entrée vide)
- [`docs/architecture.md`](architecture.md) — position de `Synthesize`
  dans le pipeline complet
- [`docs/threat-model.md`](threat-model.md) §2 — méthodologie de
  validation multi-runs (pas encore implémentée)
