# v0.8.1

## Corrective release

v0.8.1 preserves the certified v0.8 product and security semantics while
closing release-engineering gaps discovered during independent review.

## Release qualification

- The candidate is integrated into `master` before tag authorization.
- Mandatory exact-SHA gates are required before publication.
- The canonical Lima/Core authenticated UI path is scripted by
  `make ui-lima-auth`.
- The canonical bootstrap path safely reuses an existing Lima VM under
  Bash `pipefail`.

## Published artifacts

- The Operations Center/executor OCI image is published by the release
  workflow.
- The Helm chart is published with version `0.8.1` and appVersion `0.8.1`.
- Go release binaries and checksums are published through GoReleaser.

## Security and trust boundary

- Trusted-proxy HMAC authentication remains required in production mode.
- There is no replay cache or nonce. A valid signed assertion may be replayed
  within the five-minute freshness window.
- The executor cannot perform governance operations.
- The Operations Center does not directly acquire Gadget execution authority
  in the certified execution path.
- The Operations Center retains a vestigial executor-kubeconfig input for
  compatibility; removing it is post-v0.8.1 security debt.

## Known limitations

- Operations Center and executor remain single-replica/Recreate components.
- Zero-downtime and HA are not claimed.
- CRDs remain v1alpha1 and schema migration is manual/reviewed.
- Attention diagnostic volume is not explicitly bounded; this remains
  availability/UI-performance debt.
- Unsigned trusted-proxy marker and current ServiceAccount-token behavior
  remain documented residual findings.
- SBOM, provenance, and image signatures are not claimed unless independently
  published by the release workflow.

## Historical boundary

The `v0.8.0` tag and release remain immutable historical artifacts. This
release does not rewrite prior certification records.
