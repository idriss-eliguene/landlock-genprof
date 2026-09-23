# Changelog

## [0.10.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.9.0...v0.10.0) (2026-09-23)


### Features

* **operations-center:** integrate Operations Center frontend and stabilize product documentation ([#257](https://github.com/idriss-eliguene/landlock-genprof/issues/257)) ([1da5c8a](https://github.com/idriss-eliguene/landlock-genprof/commit/1da5c8a600158afe0ff729a64ef88fb01ae71785))

## [0.9.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.8.5...v0.9.0) (2026-09-18)


### Features

* **authz:** add Operations Center namespace access ([#253](https://github.com/idriss-eliguene/landlock-genprof/issues/253)) ([9a84590](https://github.com/idriss-eliguene/landlock-genprof/commit/9a845909c72b45ccd2780a568cec80a06f16cea6))
* **environment:** add Operations Center V2 foundation ([#252](https://github.com/idriss-eliguene/landlock-genprof/issues/252)) ([622e8c0](https://github.com/idriss-eliguene/landlock-genprof/commit/622e8c0f1e82a816aa493e65ec921cbb64170e9b))
* **m4:** make Operations Center V2 commercially demonstrable ([#255](https://github.com/idriss-eliguene/landlock-genprof/issues/255)) ([0245ea7](https://github.com/idriss-eliguene/landlock-genprof/commit/0245ea7aea6b31ac7ba9601d2ca8a9ea76c59df9))
* **operations-center:** converge v0.9 UX around operator state ([#256](https://github.com/idriss-eliguene/landlock-genprof/issues/256)) ([574f14f](https://github.com/idriss-eliguene/landlock-genprof/commit/574f14fc574464a3ea4a609809fe15d2f57a62ec))
* **ui:** expose Operations Center V2 context ([#254](https://github.com/idriss-eliguene/landlock-genprof/issues/254)) ([9adb5a5](https://github.com/idriss-eliguene/landlock-genprof/commit/9adb5a50e0bac6ac2b0c294c1c5661601cc145c8))


### Bug Fixes

* **harness:** converge published v0.8.5 qualification ([#250](https://github.com/idriss-eliguene/landlock-genprof/issues/250)) ([627d7d2](https://github.com/idriss-eliguene/landlock-genprof/commit/627d7d2869d8a8861c1077e962a28622e51b903a))

## [0.8.5](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.8.4...v0.8.5) (2026-09-16)


### Bug Fixes

* **harness:** isolate shared RBAC from Helm qualification ([#243](https://github.com/idriss-eliguene/landlock-genprof/issues/243)) ([3c6cf1c](https://github.com/idriss-eliguene/landlock-genprof/commit/3c6cf1c0dfb185e65b590078f2646aa403c29186))
* **harness:** load digest-pinned proxy image into kind ([#245](https://github.com/idriss-eliguene/landlock-genprof/issues/245)) ([8c913db](https://github.com/idriss-eliguene/landlock-genprof/commit/8c913dbfc50c05267e04558fc63457d56735b64b))
* **seccomp:** include runtime user setup syscalls ([#247](https://github.com/idriss-eliguene/landlock-genprof/issues/247)) ([e49c488](https://github.com/idriss-eliguene/landlock-genprof/commit/e49c488479ed938ad75de311b1933983c53a3d54))
* **test:** use envtest for concurrent proposal generation ([#249](https://github.com/idriss-eliguene/landlock-genprof/issues/249)) ([4fa8db1](https://github.com/idriss-eliguene/landlock-genprof/commit/4fa8db1d0045dfad3e873bde4d20dc5e857d6bb8))

## [0.8.4](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.8.3...v0.8.4) (2026-09-15)


### Bug Fixes

* **ci:** qualify merged master SHAs before release ([#240](https://github.com/idriss-eliguene/landlock-genprof/issues/240)) ([4ab17e8](https://github.com/idriss-eliguene/landlock-genprof/commit/4ab17e8e4752743fe0a50baddfd2b28848f6de20))

## [0.8.3](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.8.2...v0.8.3) (2026-09-15)


### Bug Fixes

* **release:** make qualification gate least-privilege and fail-closed ([#239](https://github.com/idriss-eliguene/landlock-genprof/issues/239)) ([d43c28a](https://github.com/idriss-eliguene/landlock-genprof/commit/d43c28adf1e53f5ccb936f300ff16e32593aa094))
* **test:** make UI value-flow harness repeatable ([#235](https://github.com/idriss-eliguene/landlock-genprof/issues/235)) ([a0bccb1](https://github.com/idriss-eliguene/landlock-genprof/commit/a0bccb1ea203673c4b711aefeddcdd17619c457f))
* **workbench:** restore qualified Operations Center closure ([#237](https://github.com/idriss-eliguene/landlock-genprof/issues/237)) ([5a3d276](https://github.com/idriss-eliguene/landlock-genprof/commit/5a3d2761502f2023e0b1eec46e5b8a8e7adef7f5))

## [0.8.2](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.8.1...v0.8.2) (2026-09-11)


### Features

* **release:** add immutable v0.8.1 recovery control plane ([#219](https://github.com/idriss-eliguene/landlock-genprof/issues/219)) ([a624153](https://github.com/idriss-eliguene/landlock-genprof/commit/a6241537ee79fd5b0b05e271d8ede175afc3edb4))


### Bug Fixes

* **release:** correct recovery tag fetch ([#222](https://github.com/idriss-eliguene/landlock-genprof/issues/222)) ([0bc78bc](https://github.com/idriss-eliguene/landlock-genprof/commit/0bc78bce6b7f3461edbddd61c7c2ec9cd4ecc2db))
* **release:** publish immutable recovery artifacts ([#220](https://github.com/idriss-eliguene/landlock-genprof/issues/220)) ([d51c055](https://github.com/idriss-eliguene/landlock-genprof/commit/d51c0554955aa4207f8b2b33261ff85b23bccb29))
* **test:** make published Lima harness self-contained ([#226](https://github.com/idriss-eliguene/landlock-genprof/issues/226)) ([50c5471](https://github.com/idriss-eliguene/landlock-genprof/commit/50c54717ad1ce4860c1955765423702486ee77bf))
* **workbench:** normalize discovered image identity ([#230](https://github.com/idriss-eliguene/landlock-genprof/issues/230)) ([9f11b61](https://github.com/idriss-eliguene/landlock-genprof/commit/9f11b6145b4ebe0c21f9dea3a3095e3f68f3bf89))

## [0.8.1](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.8.0...v0.8.1)

Corrective release. No product/security semantics changed from v0.8.0; this
release closes release-engineering gaps an independent adversarial review
found in how v0.8.0 was produced.

### Fixed

* The release-authorization procedure now runs to completion before a tag is
  created: mandatory CI, Core E2E, SPO Interop E2E, and SPO D-MIN E2E all
  pass on the exact commit that `v0.8.1` tags, and that commit is merged to
  `master` through a reviewed pull request, matching the process
  `CONTRIBUTING.md` has documented as mandatory since before v0.8.0.
* The Operations Center/executor container image is now built and published
  (`v0.8.0` never published one, though the Helm chart referenced it by
  default). Helm chart defaults now point at an image that actually exists.
* README.md and `book/src/workbench.md` corrected to the certified six
  top-level surfaces (Overview, Workloads, Observations, Proposals, History,
  Attention); the previous text described a stale five-surface layout that
  had already been superseded before v0.8.0 shipped.
* A production-like trusted-proxy authenticated UI qualification path is now
  scripted (`make ui-lima-auth`) rather than requiring an operator to
  hand-assemble the fixture from prose instructions.
* A gosec false positive (an environment-variable name pattern-matching the
  "hardcoded credential" rule) and a stale known-diagnostic expectation (a
  probabilistic race diagnostic reclassified to the tolerant helper already
  used for its sibling case) are corrected; neither reflects a real defect
  in v0.8.0.

### Documentation

* `docs/v08-governance-operations.md` records, without retracting it, that
  `v0.8.0` was tagged and published without completing the above gate; the
  underlying security/governance implementation was independently reviewed
  and found sound. `v0.8.1` is the release that also has correct
  release-authorization evidence and a published image.

## [0.8.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.7.0...v0.8.0)

### Added

* Governance Operations Workbench with bounded Environment, Attention,
  Observation, Proposal, and History read surfaces.
* Five independent verification axes: derivation, governance, application,
  structural application-time knowledge, and behavioral verification.
* Positive-only workload-UID ambiguity disclosure and bounded v0.8 read APIs.

### Semantic guarantees

* Approved-policy authority remains candidate-v2 and Proposal-object-scoped;
  multiple valid approvals remain ambiguous.
* History preserves timestamped events separately from untimestamped custody
  facts and exposes schema limitations.
* The browser remains read-only for governance; approval, application, and
  rollback remain outside browser authority.

### Known limitations / nonclaims

* No behavioral enforcement verification, continuous monitoring, continuous
  drift detection, or multicluster governance is provided.
* The projection does not prove complete observation coverage, complete least
  privilege, complete approval history, contribution chronology, or complete
  workload-UID history.
* Multi-object reads and apply/rollback remain best-effort/nontransactional.

This is a release candidate entry. Publication, tagging, and release assets
remain separate authorized actions.

## [0.7.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.6.1...v0.7.0) (2026-09-07)


### Features

* **history:** add idempotent observation contribution protocol ([a208b4d](https://github.com/idriss-eliguene/landlock-genprof/commit/a208b4d9a9c42c11e0cf99af5f4cb0c93ac988df))
* **history:** add observation contribution bookkeeping ([b15b739](https://github.com/idriss-eliguene/landlock-genprof/commit/b15b739a4b5fe2f27bc586a14c855c9abd256f2b))
* **history:** add population scope domain ([c9bb52b](https://github.com/idriss-eliguene/landlock-genprof/commit/c9bb52b938d7704421dabb324018c84803394786))
* **observation:** add durable observation persistence ([a86c0ce](https://github.com/idriss-eliguene/landlock-genprof/commit/a86c0ceb66ecee15fb8d2b5669d26905d2245c22))
* **observation:** add executor authority fencing ([a632bb3](https://github.com/idriss-eliguene/landlock-genprof/commit/a632bb3efb97065f58c296b292e23685a92e22e2))
* **observation:** add remaining runtime evidence sources ([890e92a](https://github.com/idriss-eliguene/landlock-genprof/commit/890e92a4ababcdbc11e55f1fff80326955a51adc))
* **observation:** add v0.7 domain contract ([55231d7](https://github.com/idriss-eliguene/landlock-genprof/commit/55231d7df8211fc1974758dc4b50482342258f70))
* **observation:** add v0.7 identity primitives ([786b14c](https://github.com/idriss-eliguene/landlock-genprof/commit/786b14caeda36125b086ebea1a3741f7f4a82ec7))
* **observation:** complete filesystem runtime observation ([d452e87](https://github.com/idriss-eliguene/landlock-genprof/commit/d452e879821ed12034abf65afd232b778f575fad))
* **observation:** enable bounded container-scoped capture ([5f52495](https://github.com/idriss-eliguene/landlock-genprof/commit/5f524952ee1b9eb2aaec0c525f2b0e50efd6c9f6))
* **observation:** persist bounded normalized runtime facts ([15f3f04](https://github.com/idriss-eliguene/landlock-genprof/commit/15f3f0450bbf10bfe1ab76bcbecf44c0fb96f5f6))
* **observation:** release v0.7 Observation Workbench ([#215](https://github.com/idriss-eliguene/landlock-genprof/issues/215)) ([929b04b](https://github.com/idriss-eliguene/landlock-genprof/commit/929b04bd415684819afd6a210e101adda77c2048))
* **observation:** resolve real target identity and truthful trace_open attach signal ([17ade9a](https://github.com/idriss-eliguene/landlock-genprof/commit/17ade9ab1108e14f5a5bee4b78d9e5a18cb17fa2))


### Bug Fixes

* **observation:** bound startup attachment waiting ([4f0d10f](https://github.com/idriss-eliguene/landlock-genprof/commit/4f0d10fb4b0f50feecdda7ecec3cb14d25b13cc6))
* **observation:** close final G5 certification findings ([8a8b6b5](https://github.com/idriss-eliguene/landlock-genprof/commit/8a8b6b523f777fc232a67e49ea4f94f501234fdb))
* **observation:** enforce lost-executor terminal recovery ([d379efe](https://github.com/idriss-eliguene/landlock-genprof/commit/d379efe87b2fc70af63dccb3283ff0b7315f0960))
* **observation:** qualify filesystem runtime events ([092cd01](https://github.com/idriss-eliguene/landlock-genprof/commit/092cd0161953ef82f5d65212f31072d8647ee024))

## [0.6.1](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.6.0...v0.6.1) (2026-09-05)


### Bug Fixes

* **dependency:** align custom Gadget build set with v0.55.1 runtime ([#213](https://github.com/idriss-eliguene/landlock-genprof/issues/213)) ([c76986e](https://github.com/idriss-eliguene/landlock-genprof/commit/c76986ed1ec5408101c20482cc81f359b4597c8b))
* **workbench:** reproducible v0.6.1 contributor bootstrap ([#211](https://github.com/idriss-eliguene/landlock-genprof/issues/211)) ([8fbe45b](https://github.com/idriss-eliguene/landlock-genprof/commit/8fbe45b648ee4c25565e7532a9163b38cba853c5))

## [0.6.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.5.2...v0.6.0) (2026-09-04)


### Features

* **workbench:** add full visual inspection experience ([#209](https://github.com/idriss-eliguene/landlock-genprof/issues/209)) ([96cc07d](https://github.com/idriss-eliguene/landlock-genprof/commit/96cc07de1ad6186442890acf1be53fe0cf0e588f))

## [0.5.2](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.5.1...v0.5.2) (2026-09-04)


### Features

* **rollback:** add explicit governed rollback ([#203](https://github.com/idriss-eliguene/landlock-genprof/issues/203)) ([897ebfd](https://github.com/idriss-eliguene/landlock-genprof/commit/897ebfdae63eeb468c7b38c7eca63e85d8596538))


### Miscellaneous Chores

* **release:** select v0.5.2 ([#207](https://github.com/idriss-eliguene/landlock-genprof/issues/207)) ([ae125c4](https://github.com/idriss-eliguene/landlock-genprof/commit/ae125c4bb50950eb0d9ecf4bfad56be360da59d0))

## [0.5.1](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.5.0...v0.5.1) (2026-09-03)


### Features

* **applyattempt:** add durable mutation custody ([#202](https://github.com/idriss-eliguene/landlock-genprof/issues/202)) ([8da8637](https://github.com/idriss-eliguene/landlock-genprof/commit/8da8637774aa379110432fef4677ce38006e521f))

## [0.5.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.4.0...v0.5.0) (2026-09-02)


### Features

* **dashborad:** [v0.5.0][G4] Build cluster-centric read-only Workbench experience ([#199](https://github.com/idriss-eliguene/landlock-genprof/issues/199)) ([5881f98](https://github.com/idriss-eliguene/landlock-genprof/commit/5881f9807395d1eee8e89426f32110867f41e8fb))
* **governance:** certify workload associations ([#192](https://github.com/idriss-eliguene/landlock-genprof/issues/192)) ([628a5e1](https://github.com/idriss-eliguene/landlock-genprof/commit/628a5e1e86b7b6f28de4a26d6a209e4c675cbf2c))
* **governance:** persist canonical target provenance ([#195](https://github.com/idriss-eliguene/landlock-genprof/issues/195)) ([5bd01fc](https://github.com/idriss-eliguene/landlock-genprof/commit/5bd01fc92f0b51bf5662eab91a9ed1d55e003e30))
* **identity:** add canonical governed target semantics ([#179](https://github.com/idriss-eliguene/landlock-genprof/issues/179)) ([d98ee1b](https://github.com/idriss-eliguene/landlock-genprof/commit/d98ee1b3195c8f40c72b2000a287789ae96941af))
* **k8s:** add bounded cluster workload read model ([#190](https://github.com/idriss-eliguene/landlock-genprof/issues/190)) ([9ec7f4b](https://github.com/idriss-eliguene/landlock-genprof/commit/9ec7f4bcceefc66f8acf1a0a743faad9dad63f63))
* **k8s:** add bounded Workbench read capability ([#188](https://github.com/idriss-eliguene/landlock-genprof/issues/188)) ([211f1b6](https://github.com/idriss-eliguene/landlock-genprof/commit/211f1b68e93f88fc8d1a341c5e832a145eea03f2))
* **workbench:** add workload security projections ([#196](https://github.com/idriss-eliguene/landlock-genprof/issues/196)) ([9f7d04a](https://github.com/idriss-eliguene/landlock-genprof/commit/9f7d04a7ab6cb1bcbecce6eb6b3fbff94dfcae39))
* **workbench:** establish the G3 local HTTP trust boundary ([#197](https://github.com/idriss-eliguene/landlock-genprof/issues/197)) ([34d5684](https://github.com/idriss-eliguene/landlock-genprof/commit/34d568424bdb06827720633f387afe09f7f6c16b))

## [0.4.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.3.0...v0.4.0) (2026-08-31)


### Features

* **ui:** [v0.4.0][G2] expand Workbench review views ([#168](https://github.com/idriss-eliguene/landlock-genprof/issues/168)) ([c9f61f9](https://github.com/idriss-eliguene/landlock-genprof/commit/c9f61f9b89735449ba458365b4f83310df30d6a2))
* **ui:** [v0.4.0][G2] prove or kill a vertical-slice local Workbench ([#164](https://github.com/idriss-eliguene/landlock-genprof/issues/164)) ([408790a](https://github.com/idriss-eliguene/landlock-genprof/commit/408790ad871d812f9b626b826f407f347650f69c))


### Bug Fixes

* **history:** partition confidence by evidence population ([#175](https://github.com/idriss-eliguene/landlock-genprof/issues/175)) ([504ea40](https://github.com/idriss-eliguene/landlock-genprof/commit/504ea40c66cbd654c5056ff85186f4971ff00a82))

## [0.3.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.2.3...v0.3.0) (2026-08-27)


### Features

* **spo:** add governed PodLock and SPO security profile convergence ([#154](https://github.com/idriss-eliguene/landlock-genprof/issues/154)) ([089796a](https://github.com/idriss-eliguene/landlock-genprof/commit/089796a2addafc74d3b673aa38f95b300f43e923))

## [0.2.3](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.2.2...v0.2.3) (2026-08-20)


### Bug Fixes

* **docs:** repair current architecture diagram ([#146](https://github.com/idriss-eliguene/landlock-genprof/issues/146)) ([7a94396](https://github.com/idriss-eliguene/landlock-genprof/commit/7a9439615c34cb945386e8eb6b9fbb295adf7886))

## [0.2.2](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.2.1...v0.2.2) (2026-08-20)


### Bug Fixes

* **docs:** align documentation with governed policy lifecycle ([#144](https://github.com/idriss-eliguene/landlock-genprof/issues/144)) ([fc3a5d9](https://github.com/idriss-eliguene/landlock-genprof/commit/fc3a5d96f5d1ab151f439919c63fe2ae8b11cbe9))

## [0.2.1](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.2.0...v0.2.1) (2026-08-20)


### Bug Fixes

* **docs:** refresh canonical v0.2 demo recording ([#141](https://github.com/idriss-eliguene/landlock-genprof/issues/141)) ([dd6a190](https://github.com/idriss-eliguene/landlock-genprof/commit/dd6a1906e8e37a11d89d85e322b2ff624c69cab5))

## [0.2.0](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.1.3...v0.2.0) (2026-08-20)


### Features

* **proposal:** add an approval status lifecycle to SecurityProfileProposal ([#131](https://github.com/idriss-eliguene/landlock-genprof/issues/131)) ([569a75b](https://github.com/idriss-eliguene/landlock-genprof/commit/569a75b22a598a9f7a8415c8a75ed8ead842d21d))
* **proposal:** release governed proposal approval and apply workflow ([#138](https://github.com/idriss-eliguene/landlock-genprof/issues/138)) ([9c57d6d](https://github.com/idriss-eliguene/landlock-genprof/commit/9c57d6d7025ca99e989375eef2551e9b45147786))


### Bug Fixes

* **ci:** add workflow_dispatch fallback for tag-triggered releases ([#122](https://github.com/idriss-eliguene/landlock-genprof/issues/122)) ([3dda9d4](https://github.com/idriss-eliguene/landlock-genprof/commit/3dda9d464a8851d315b0c231305a03cdec4c8de9))
* **ci:** docs-release.yml shouldn't check out the target tag itself ([#125](https://github.com/idriss-eliguene/landlock-genprof/issues/125)) ([313b3b3](https://github.com/idriss-eliguene/landlock-genprof/commit/313b3b3e5eac9663f37c63b6dfc7a047811c00f1))
* **ci:** use a PAT for release-please, not the default token ([#124](https://github.com/idriss-eliguene/landlock-genprof/issues/124)) ([cc3ede3](https://github.com/idriss-eliguene/landlock-genprof/commit/cc3ede39d32fe3fd9a3d617dad3587608e4e0066))

## [0.1.3](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.1.2...v0.1.3) (2026-08-01)


### Features

* **docs:** add build tooling for a versioned docs site ([eb225da](https://github.com/idriss-eliguene/landlock-genprof/commit/eb225da4482a923a386730afc328886ca252abe9))


### Bug Fixes

* **release:** force next release to 0.1.3, not the spuriously-computed 0.2.0 ([#121](https://github.com/idriss-eliguene/landlock-genprof/issues/121)) ([729f038](https://github.com/idriss-eliguene/landlock-genprof/commit/729f0387b5223703ce20be6df2c918e7d8890b7f))

## [0.1.2](https://github.com/idriss-eliguene/landlock-genprof/compare/v0.1.1...v0.1.2) (2026-07-31)


### Bug Fixes

* **doc:** auto-sync version refs via release-please, enforce PR-title-driven releases ([#96](https://github.com/idriss-eliguene/landlock-genprof/issues/96)) ([831a086](https://github.com/idriss-eliguene/landlock-genprof/commit/831a086a8570f893ed0bff813643cb21289e0282))
