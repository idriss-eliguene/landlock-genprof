# DOC-R3 Screenshot Environment

## Canonical environment

- Repository: `/Users/idrisseliguene/workdir/landlock-genprof`
- Git SHA: `0649068ebda54b0f57e8e8a14360fb7b723ab1f2`
- Lima VM: `landlock-genprof-core`
- Kubernetes context: `kind-landlock-genprof-core`
- Cluster identity observed in the DOC-R3 live capture: `9c8cb4cd-7316-49b2-a5b2-75f43c7d667a`
- Namespace: `payments`
- Authenticated actor: `qualification-operator`
- Canonical route: `http://127.0.0.1:8095/` through the repository trusted-proxy fixture
- Runtime: kind + Cilium; Cilium `1.20.1`; Inspektor Gadget `v0.55.1`
- Capture viewport: `1920x1080` for the new overview capture; existing captures retain their documented source viewport.

The new overview frame was captured from the real authenticated React
Operations Center. The remaining six frames are reused authentic Playwright
captures already documented by the repository's Operations Center screenshot
record; they were selected because the live harness's bundled smoke driver
failed before establishing a stable workload flow in this run. No rendered
content or backend response was edited.

The disposable authenticated harness and its temporary RBAC, executor, proxy,
and HMAC materials are runtime fixtures only. They are not documentation
authority and are not committed.
