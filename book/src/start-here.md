# Choose your path

landlock-genprof has different prerequisites depending on what you want to do. Start with the path that matches your goal; you do not need to read the documentation in order.

<div class="doc-path-grid">
  <a class="doc-path" href="docs/test-environment.html"><span>Evaluate</span><strong>I want a disposable test cluster</strong><small>Create a kind cluster, install the tool, and produce a first result.</small></a>
  <a class="doc-path" href="INSTALL.html"><span>Operate</span><strong>I already have a Kubernetes cluster</strong><small>Choose a CLI or Helm installation path and check cluster prerequisites.</small></a>
  <a class="doc-path" href="demo/index.html"><span>Understand</span><strong>I want to see the complete flow</strong><small>Run the reproducible demonstration from observation through governed apply.</small></a>
  <a class="doc-path" href="HOW_TO_START.html"><span>Contribute</span><strong>I want to work on the code</strong><small>Set up a Linux development environment and follow the contributor workflow.</small></a>
</div>

## Before you begin

The product observes runtime behavior and turns it into a reviewable policy candidate. Observation is not authorization, and generation is not enforcement. The normal lifecycle is:

```text
observe → synthesize → review → approve → apply → verify
```

Read [the governed workflow](workflow.md) for the responsibility of each stage. Before testing enforcement, check the [enforcement prerequisites](docs/enforcement-prerequisites.md): filesystem, network, and syscall policy depend on separate enforcement mechanisms.

## Find the right kind of documentation

| If you need… | Go to… |
|---|---|
| A task-oriented walkthrough | [Usage guide](docs/usage.md) |
| Exact flags and command syntax | [CLI reference](cli/landlock-genprof.md) |
| Components and data flow | [Architecture overview](docs/architecture.md) |
| Security assumptions and limitations | [Security model](docs/threat-model.md) |
| Proof of what works today | [Demonstrated capabilities](project/progress.md) |
| Intended future capabilities | [Product roadmap](project/roadmap.md) |

> **Status vocabulary:** “generated,” “applied,” “enforced,” and “verified” are distinct claims. The progress page is the canonical record of demonstrated behavior; the roadmap describes intent and sequencing.
