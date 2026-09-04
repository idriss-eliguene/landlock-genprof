<div class="lg-landing">
<link rel="stylesheet" href="css/landing.css">
<section class="lg-hero">
  <div class="lg-wrap">
    <p class="lg-eyebrow">Govern runtime-derived Kubernetes policy</p>
    <h1 class="lg-headline">Learn what it needs.<br>Authorize <em>exactly</em> what you reviewed.</h1>
    <p class="lg-lede">Runtime learning produces knowledge, not deployment authority. <strong>landlock-genprof</strong> combines direct filesystem/network evidence with internal or SPO-derived seccomp policy, builds one reviewable candidate, binds human approval to its exact digest, and applies only what remains authorized.</p>
    <p class="lg-status">Status: the observe → synthesize → review → approve → apply pipeline is built, with approval bound to the exact reviewed candidate, tagged <code class="lg-inline-code">v0.5.2</code><!-- x-release-please-version -->. The current source also includes the v0.6 Full Visual Workbench: a workload-first, read-only inspection surface for the same governed identity and custody records. Backend-specific evidence is scoped: <code class="lg-inline-code">NetworkPolicy</code> denial is demonstrated on Cilium, and the SPO/Seccomp behavioral denial boundary is demonstrated for the tested candidate/runtime path; PodLock/Landlock kernel denial remains unproven. Artifact application or backend readiness is not itself enforcement evidence. <strong>Same-Pod PodLock + SPO runtime compatibility and complete same-Pod non-interference are NOT CERTIFIED.</strong> These results do not establish universal compatibility across kernels, Kubernetes versions, runtimes, CNIs, SPO versions, or PodLock versions. <a href="project/progress.html">Progress</a> is the canonical record of what's demonstrated, capability by capability.</p>
    <div class="lg-cta-row">
      <a class="lg-btn lg-primary" href="#lg-loop">Try it in 4 commands</a>
      <a class="lg-btn lg-ghost" href="start-here.html">Choose your path</a>
    </div>
    <div class="lg-survey lg-ticked">
      <div class="lg-plate-label"><span>PLAT · nginx-demo / default</span><span>observed 60s</span></div>
      <svg viewBox="0 0 760 300" role="img" aria-labelledby="lgSurveyTitle">
        <title id="lgSurveyTitle">A loose, hand-guessed permission boundary compared against a tight boundary drawn around the paths, ports, and syscalls actually observed during a training run.</title>
        <polygon points="60,40 700,30 730,150 660,270 120,260 40,150" fill="none" stroke="var(--lg-line)" stroke-width="1.6" stroke-dasharray="7 6" />
        <text x="60" y="24" font-family="var(--lg-font-mono)" font-size="12" fill="var(--lg-ink-soft)" letter-spacing="0.04em">HAND-GUESSED — everything, just in case</text>
        <polygon points="330,110 420,95 470,130 480,175 440,205 360,210 305,175 300,135" fill="var(--lg-accent)" fill-opacity="0.09" stroke="var(--lg-accent)" stroke-width="2.4" />
        <text x="305" y="230" font-family="var(--lg-font-mono)" font-size="12" fill="var(--lg-accent)" letter-spacing="0.04em">OBSERVED &amp; GENERATED</text>
        <g fill="var(--lg-ink-soft)">
          <circle cx="345" cy="130" r="3" /><circle cx="392" cy="118" r="3" /><circle cx="430" cy="140" r="3" />
          <circle cx="455" cy="165" r="3" /><circle cx="420" cy="185" r="3" /><circle cx="375" cy="190" r="3" />
          <circle cx="335" cy="170" r="3" /><circle cx="322" cy="145" r="3" /><circle cx="400" cy="155" r="3" />
        </g>
      </svg>
      <div class="lg-survey-legend">
        <span class="lg-item"><span class="lg-swatch lg-loose"></span> broad, hand-authored — never trimmed back</span>
        <span class="lg-item"><span class="lg-swatch lg-tight"></span> direct evidence, confidence-annotated where applicable</span>
      </div>
    </div>
  </div>
</section>
<section class="lg-section" id="lg-authority">
  <div class="lg-wrap">
    <div class="lg-section-head"><h2>LEARNED &ne; AUTHORIZED</h2><span class="lg-num">the boundary</span></div>
    <p class="lg-section-note">Runtime learning is a solved problem, and other systems do it better. <a href="https://github.com/kubernetes-sigs/security-profiles-operator">security-profiles-operator</a> records syscalls with a production eBPF recorder, generates a <code class="lg-inline-code">SeccompProfile</code>, installs it on every node and enforces it.</p>
    <p class="lg-section-note">What no learner provides is a <strong>decision</strong>. A recorded profile describes what a workload <em>did</em>; enforcing it is a statement about what it is <em>allowed</em> to do. Those are not the same claim.</p>
    <p class="lg-section-note"><strong>Direct observation:</strong> landlock-genprof acquires filesystem, network, and applicable capability evidence. <strong>SPO-derived policy:</strong> in SPO mode, Security Profiles Operator owns syscall observation and produces the real <code class="lg-inline-code">SeccompProfile</code>; landlock-genprof imports it as derived policy with provenance preserved.</p>
    <p class="lg-section-note">Different origins converge in one reviewable <code class="lg-inline-code">SecurityProfileProposal</code>. SPO-derived syscalls do not enter landlock-genprof <code class="lg-inline-code">TrainingHistory</code> and receive no invented confidence.</p>
    <p class="lg-section-note"><code class="lg-inline-code">CandidateDigest</code> is deterministic content identity, <strong>not authority</strong>. Human approval binds authority to that exact digest; changed content cannot inherit stale approval. See the <a href="workflow.html">governed workflow</a> and <a href="docs/adr/0008-spo-derived-policy-import-boundary.html">ADR-0008</a>.</p>
  </div>
</section>
<section class="lg-section" id="lg-loop">
  <div class="lg-wrap">
    <div class="lg-section-head"><h2>Observe, review, approve, apply</h2><span class="lg-num">the governed loop</span></div>
    <p class="lg-section-note">Four commands, in this order. Approval is bound to the exact candidate digest. Governed apply revalidates that authority and implemented backend readiness, refusing missing, stale, or mismatched approval. External systems enforce: <strong>applied &ne; enforced; enforced &ne; verified.</strong></p>
    <div class="lg-loop">
      <div class="lg-step">
        <span class="lg-verb">01 — trace</span>
        <h3>Watch it run</h3>
        <p>Collects direct evidence for the selected source mode and publishes a candidate. In SPO mode, syscall policy comes from the named SPO-derived <code class="lg-inline-code">SeccompProfile</code>.</p>
        <pre><code>kubectl landlock-genprof trace \
  --pod nginx-demo -n default \
  --binary /usr/sbin/nginx \
  --duration 60s</code></pre>
      </div>
      <div class="lg-step">
        <span class="lg-verb">02 — review</span>
        <h3>See what it saw</h3>
        <p>Prints the mixed-origin candidate, preserved provenance, applicable confidence, artifact readiness, and the <strong>CandidateDigest</strong> identifying its exact content.</p>
        <pre><code>kubectl landlock-genprof review \
  nginx-demo</code></pre>
      </div>
      <div class="lg-step">
        <span class="lg-verb">03 — approve</span>
        <h3>Authorize that exact candidate</h3>
        <p>Approval binds to the digest <code class="lg-inline-code">review</code> printed, not to the proposal's name — so a later run that changes the candidate does not inherit it.</p>
        <pre><code>kubectl landlock-genprof approve \
  nginx-demo \
  --expected-digest sha256:&lt;from-review&gt;</code></pre>
      </div>
      <div class="lg-step">
        <span class="lg-verb">04 — apply-proposal</span>
        <h3>Apply only what was approved</h3>
        <p>Re-reads the proposal and re-checks the digest before applying anything; a missing, stale, or changed binding fails closed. The confirmation prompt is an extra operator safeguard on top, not the authority gate.</p>
        <pre><code>kubectl landlock-genprof \
  apply-proposal nginx-demo</code></pre>
      </div>
    </div>
  </div>
</section>
<section class="lg-section" id="lg-demo">
  <div class="lg-wrap">
    <div class="lg-section-head"><h2>See it run</h2><span class="lg-num">real recording, not staged</span></div>
    <p class="lg-section-note">A short interactive capture from a live cluster. For the complete current SPO-derived-policy, digest-bound approval, stale-authority rejection, and governed-apply scenario, follow <a href="demo/index.html">the canonical demo</a>.</p>
    <div class="lg-survey lg-ticked">
      <div class="lg-plate-label"><span>RECORDING · nginx-demo / default</span><span>click to play on asciinema →</span></div>
      <a href="https://asciinema.org/a/Y0IHrGK0zYcDbgaw"><img src="demo/demo.gif" alt="landlock-genprof live cluster recording" /></a>
    </div>
  </div>
</section>
<section class="lg-section">
  <div class="lg-wrap">
    <div class="lg-section-head"><h2>One candidate, four domains</h2><span class="lg-num">what gets governed</span></div>
    <p class="lg-section-note">Direct observations can carry cross-run confidence. SPO-derived syscalls are different: they enter as derived policy with provenance, never as landlock-genprof observations, and receive no fabricated TrainingHistory confidence.</p>
    <div class="lg-parcels">
      <div class="lg-parcel lg-ticked">
        <span class="lg-domain">Filesystem</span><h3>Landlock policy</h3>
        <span class="lg-format">→ PodLock LandlockProfile</span>
        <span class="lg-conf lg-high"><span class="lg-dot"></span>seen on every run</span>
      </div>
      <div class="lg-parcel lg-ticked">
        <span class="lg-domain">Network</span><h3>Egress rights</h3>
        <span class="lg-format">→ Kubernetes NetworkPolicy</span>
        <span class="lg-conf lg-high"><span class="lg-dot"></span>seen on every run</span>
      </div>
      <div class="lg-parcel lg-ticked">
        <span class="lg-domain">Syscalls</span><h3>Seccomp profile</h3>
        <span class="lg-format">→ security-profiles-operator CR</span>
        <span class="lg-conf lg-medium"><span class="lg-dot"></span>SPO-derived: provenance, no invented confidence</span>
      </div>
      <div class="lg-parcel lg-ticked">
        <span class="lg-domain">Capabilities</span><h3>Linux capabilities</h3>
        <span class="lg-format">→ securityContext fragment</span>
        <span class="lg-conf lg-low"><span class="lg-dot"></span>review before prod</span>
      </div>
    </div>
  </div>
</section>
<section class="lg-section" id="lg-start">
  <div class="lg-wrap">
    <div class="lg-section-head"><h2>Where to start</h2><span class="lg-num">pick one</span></div>
    <div class="lg-paths">
      <a class="lg-path-card lg-ticked" href="docs/test-environment.html">
        <span class="lg-tag">No cluster yet</span><h3>Set up a test environment</h3>
        <p>A disposable kind cluster, from nothing — one script.</p>
        <span class="lg-go">docs/test-environment →</span>
      </a>
      <a class="lg-path-card lg-ticked" href="INSTALL.html">
        <span class="lg-tag">Already have one</span><h3>Install</h3>
        <p>Get the CLI, apply the RBAC/CRDs, against a cluster you already run.</p>
        <span class="lg-go">INSTALL →</span>
      </a>
      <a class="lg-path-card lg-ticked" href="docs/usage.html">
        <span class="lg-tag">Reference</span><h3>Usage &amp; CLI reference</h3>
        <p>Every flag, one section each — plus a generated page per command.</p>
        <span class="lg-go">docs/usage →</span>
      </a>
      <a class="lg-path-card lg-ticked" href="docs/architecture.html">
        <span class="lg-tag">Under the hood</span><h3>Architecture</h3>
        <p>Components and interactions, at a glance — deep dives nested underneath.</p>
        <span class="lg-go">docs/architecture →</span>
      </a>
    </div>
    <div class="lg-note-callout">
      <strong>Working on the codebase itself?</strong>
      See the <a href="HOW_TO_START.html">contributor quickstart</a> instead — git workflow, code walkthrough, first tasks per role.
    </div>
  </div>
</section>
<section class="lg-section">
  <div class="lg-wrap">
    <div class="lg-section-head"><h2>Complementary, not competing</h2><span class="lg-num">positioning</span></div>
    <p class="lg-section-note">landlock-genprof doesn't implement the kernel enforcement mechanisms itself — it feeds three existing, independent backends, one per domain, in the format each expects. None of them are installed by this project, and <strong>generating or applying an artifact is not the same as enforcing it</strong>: each card below says how far v0.2.0 actually goes. What each mechanism needs: <a href="docs/enforcement-prerequisites.html">enforcement prerequisites</a>. Full positioning against PodLock/SPO/static compliance scanners: <a href="docs/product-definition-v1.html">product definition</a>.</p>
    <div class="lg-parcels">
      <div class="lg-parcel">
        <span class="lg-domain">Filesystem (Landlock)</span><h3>PodLock</h3>
        <span class="lg-format">Kubewarden ecosystem — enforces at container startup</span>
        <span class="lg-conf lg-medium"><span class="lg-dot"></span>generated · approval-bound · API applied</span>
        <span class="lg-conf lg-low"><span class="lg-dot"></span>kernel enforcement not demonstrated in v0.2.0</span>
      </div>
      <div class="lg-parcel">
        <span class="lg-domain">Syscalls (seccomp)</span><h3>security-profiles-operator</h3>
        <span class="lg-format">materializes the profile onto every node</span>
        <span class="lg-conf lg-medium"><span class="lg-dot"></span>generated · API plumbing tested</span>
        <span class="lg-conf lg-high"><span class="lg-dot"></span>tested Seccomp boundary demonstrated on a real node</span>
        <span class="lg-conf lg-low"><span class="lg-dot"></span>not a universal least-privilege claim</span>
      </div>
      <div class="lg-parcel">
        <span class="lg-domain">Network</span><h3>Your CNI</h3>
        <span class="lg-format">any implementation of NetworkPolicy</span>
        <span class="lg-conf lg-high"><span class="lg-dot"></span>generated · approval-bound · API applied</span>
        <span class="lg-conf lg-high"><span class="lg-dot"></span>enforcement demonstrated on Cilium</span>
        <span class="lg-conf lg-low"><span class="lg-dot"></span>that result is Cilium-specific — not all CNIs</span>
      </div>
    </div>
  </div>
</section>
</div>
