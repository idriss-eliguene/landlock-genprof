# The governed workflow

Runtime knowledge is useful, but it is not deployment authority. landlock-genprof brings direct evidence and derived policy into one candidate, gives that candidate deterministic content identity, and applies it only after a human authorizes that exact identity.

<div class="gov-flow" aria-label="Knowledge sources become a candidate, receive explicit authority, and proceed through governed apply">
  <section class="gov-stage gov-sources">
    <span class="gov-kicker">01 · Knowledge sources</span>
    <div class="gov-source">
      <strong>Direct evidence</strong>
      <p>landlock-genprof acquires filesystem, network, and applicable capability evidence.</p>
    </div>
    <div class="gov-source gov-source-spo">
      <strong>SPO-derived policy</strong>
      <p>Security Profiles Operator observes syscalls and produces the real derived <code>SeccompProfile</code>.</p>
    </div>
    <p class="gov-caption">Different origins. One candidate.</p>
  </section>
  <section class="gov-stage">
    <span class="gov-kicker">02 · Candidate</span>
    <strong class="gov-object">SecurityProfileProposal</strong>
    <p>Direct evidence and the imported SPO artifact converge with provenance preserved.</p>
    <strong class="gov-object">CandidateDigest</strong>
    <p>Deterministic content identity. <em>Not authority.</em></p>
  </section>
  <section class="gov-stage gov-authority">
    <span class="gov-kicker">03 · Authorization</span>
    <div class="gov-stack"><strong>Reviewed content</strong><span>↓</span><strong>Exact digest</strong><span>↓</span><strong>Human approval</strong></div>
    <p><code>review</code> exposes the candidate. <code>approve</code> binds authority to that digest only. Changed content cannot inherit an earlier approval.</p>
  </section>
  <section class="gov-stage">
    <span class="gov-kicker">04 · Apply</span>
    <strong class="gov-object">apply-proposal</strong>
    <p>Re-reads and revalidates the proposal, re-checks approval, checks implemented backend readiness, and refuses missing, stale, or mismatched authority.</p>
    <div class="gov-invariants"><strong>APPLIED ≠ ENFORCED</strong><strong>ENFORCED ≠ VERIFIED</strong></div>
  </section>
</div>

In SPO mode, the imported `SeccompProfile` is derived policy—not landlock-genprof observation. Its provenance is preserved, its syscalls do not enter landlock-genprof `TrainingHistory`, and landlock-genprof invents no confidence for them. The source object grants no authority; the governed candidate still requires normal digest-bound approval. See [Import SPO-derived policy](docs/usage/spo-seccomp-import.md) and [ADR-0008](docs/adr/0008-spo-derived-policy-import-boundary.md).

## Four commands, one authority chain

<div class="gov-command-grid">
  <section class="gov-command">
    <span class="gov-command-number">01</span>
    <h3>trace</h3>
    <p>Collect direct evidence for the selected source mode and publish a candidate. In SPO mode, filesystem and network evidence remain landlock-genprof-derived while syscall policy comes from the named SPO-derived <code>SeccompProfile</code>.</p>
    <pre><code>kubectl landlock-genprof trace \
  --pod nginx-demo -n default \
  --binary /usr/sbin/nginx \
  --duration 60s</code></pre>
  </section>
  <section class="gov-command">
    <span class="gov-command-number">02</span>
    <h3>review</h3>
    <p>Inspect the exact mixed-origin candidate, source provenance, applicable confidence, artifact readiness, and the candidate digest.</p>
    <pre><code>kubectl landlock-genprof review \
  nginx-demo</code></pre>
  </section>
  <section class="gov-command">
    <span class="gov-command-number">03</span>
    <h3>approve</h3>
    <p>Record explicit human authority for the digest printed by <code>review</code>. A later candidate must be reviewed and approved again.</p>
    <pre><code>kubectl landlock-genprof approve \
  nginx-demo \
  --expected-digest sha256:&lt;from-review&gt;</code></pre>
  </section>
  <section class="gov-command">
    <span class="gov-command-number">04</span>
    <h3>apply-proposal</h3>
    <p>Apply only a valid, current approval. Enforcement remains the responsibility of PodLock/Landlock, the CNI, and SPO with the kubelet/runtime.</p>
    <pre><code>kubectl landlock-genprof \
  apply-proposal nginx-demo</code></pre>
  </section>
</div>

> Before tracing, use `kubectl landlock-genprof doctor` for host prerequisites. After application, use backend-specific checks to distinguish API application, enforcement, and behavioral verification. The [usage guide](docs/usage.md) documents the complete lifecycle and the narrower semantics of `explain`, `diff`, and `verify`.

## Continue

- [Install and check prerequisites](INSTALL.md)
- [Use the complete governed lifecycle](docs/usage.md)
- [Check external enforcement prerequisites](docs/enforcement-prerequisites.md)
- [See demonstrated capabilities and limitations](project/progress.md)
