# Summary

[Home](index.md)

# Try the Product

- [Set up a test environment](docs/test-environment.md)
- [Install (existing cluster)](INSTALL.md)
- [Usage reference](docs/usage.md)
  - [Step 5 — Optional NetworkPolicy generation](docs/usage/network-policy.md)
  - [Step 6 — Optional target restart (`--restart`)](docs/usage/target-restart.md)
  - [Step 7 — Optional multi-run history (`--history`)](docs/usage/multi-run-history.md)
  - [Step 8 — Optional seccomp profile generation (`--seccomp-out`)](docs/usage/seccomp-profile.md)
  - [Step 9 — Optional Linux capabilities fragment (`--capabilities-out`)](docs/usage/capabilities-fragment.md)
  - [Step 10 — Optional composed securityContext (`--security-context-out`)](docs/usage/composed-security-context.md)
  - [Step 11 — Optional unified review report (`--report-out`)](docs/usage/unified-report.md)
  - [Step 12 — Proposal publishing (mandatory)](docs/usage/proposal-publishing.md)
  - [Step 13 — Optional ready-to-apply patched manifest (`--patched-manifest-out`)](docs/usage/patched-manifest.md)
  - [Step 14 — Optional SeccompProfile custom resource (`--seccomp-profile-out`)](docs/usage/seccompprofile-resource.md)
- [CLI reference](cli/landlock-genprof.md)
  - [trace](cli/landlock-genprof_trace.md)
  - [review](cli/landlock-genprof_review.md)
  - [apply-proposal](cli/landlock-genprof_apply-proposal.md)
  - [version](cli/landlock-genprof_version.md)

# Architecture

- [Architecture](docs/architecture.md)
  - [Full data-flow diagram (deep dive)](docs/data-flow-diagram.md)
  - [Sequence diagram (deep dive)](docs/sequence-diagram.md)
  - [Package dependencies (deep dive)](docs/packages.md)
  - [Policy synthesis (deep dive)](docs/policy-synthesis.md)

# Key Concepts

- [Threat model](docs/threat-model.md)
- [Enforcement prerequisites](docs/enforcement-prerequisites.md)

- [Roadmap](docs/roadmap.md)
  - [Product definition v1](docs/product-definition-v1.md)
  - [Product design v1](docs/product-design-v1.md)
  - [Product roadmap v1](docs/product-roadmap-v1.md)
  - [Workload review screen v1](docs/product-screen-workload-review-v1.md)
  - [Course context / pedagogy](docs/pedagogy.md)

# Contribute

- [Quickstart (English)](HOW_TO_START.md)
- [Guide de démarrage (Français)](COMMENT_COMMENCER.md)
- [Contributing](CONTRIBUTING.md)
  - [Governance](GOVERNANCE.md)
  - [Code of Conduct](CODE_OF_CONDUCT.md)
  - [DCO](DCO.md)
  - [Maintainers](MAINTAINERS.md)
