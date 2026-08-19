<div class="lg-landing">
<link rel="stylesheet" href="css/landing.css">
<section class="lg-hero">
  <div class="lg-wrap">
    <p class="lg-eyebrow">Kubernetes security profile generator</p>
    <h1 class="lg-headline">Observe the pod.<br>Draw the <em>tightest</em> boundary that fits.</h1>
    <p class="lg-lede">Landlock, seccomp, <code class="lg-inline-code">NetworkPolicy</code>, and Linux capabilities policies are normally guessed by hand, before anyone has watched the app run. <strong>landlock-genprof</strong> watches first — a training run, then a profile sized to exactly what it saw.</p>
    <p class="lg-status">Status: the observe → synthesize → review → approve → apply pipeline is built, with approval bound to the exact reviewed candidate, tagged <code class="lg-inline-code">v0.2.0</code><!-- x-release-please-version -->. Behavioral enforcement has been demonstrated for <code class="lg-inline-code">NetworkPolicy</code> on Cilium only — PodLock/Landlock and SPO seccomp enforcement have not. <a href="project/progress.html">Progress</a> is the canonical record of what's demonstrated, capability by capability.</p>
    <div class="lg-cta-row">
      <a class="lg-btn lg-primary" href="#lg-loop">Try it in 4 commands</a>
      <a class="lg-btn lg-ghost" href="#lg-start">Where do I start?</a>
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
        <span class="lg-item"><span class="lg-swatch lg-tight"></span> generated from what actually ran, confidence-annotated</span>
      </div>
    </div>
  </div>
</section>
<section class="lg-section" id="lg-loop">
  <div class="lg-wrap">
    <div class="lg-section-head"><h2>Observe, review, approve, apply</h2><span class="lg-num">the governed loop</span></div>
    <p class="lg-section-note">Four commands, in this order, every time. Approval is not optional and it is not a formality: <code class="lg-inline-code">apply-proposal</code> is bound to the exact candidate digest you approved, and refuses to apply anything else.</p>
    <div class="lg-loop">
      <div class="lg-step">
        <span class="lg-verb">01 — trace</span>
        <h3>Watch it run</h3>
        <p>Trains on the target pod for a set duration, capturing filesystem, network, syscall, and capability activity via eBPF.</p>
        <pre><code>kubectl landlock-genprof trace \
  --pod nginx-demo -n default \
  --binary /usr/sbin/nginx \
  --duration 60s</code></pre>
      </div>
      <div class="lg-step">
        <span class="lg-verb">02 — review</span>
        <h3>See what it saw</h3>
        <p>Prints what was observed, what's confident vs. not, which artifacts are ready — and the <strong>candidate digest</strong> identifying this exact candidate.</p>
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
    <p class="lg-section-note">Captured against a live cluster — trace with real traffic, the generated profile, the review summary, the raw <code class="lg-inline-code">SecurityProfileProposal</code> object, and <code class="lg-inline-code">apply-proposal --restart</code>. Recorded before the digest-bound <code class="lg-inline-code">approve</code> step became part of the loop, so step 03 above is not in this recording. Click it for the interactive version.</p>
    <div class="lg-survey lg-ticked">
      <div class="lg-plate-label"><span>RECORDING · nginx-demo / default</span><span>click to play on asciinema →</span></div>
      <a href="https://asciinema.org/a/UApURycj8LhCFnYA"><img src="demo/demo.gif" alt="landlock-genprof: trace, review, the SecurityProfileProposal object, and apply-proposal --restart against a real cluster — a real run predating the v0.2 canonical demo" /></a>
    </div>
  </div>
</section>
<section class="lg-section">
  <div class="lg-wrap">
    <div class="lg-section-head"><h2>One training run, four domains</h2><span class="lg-num">what gets generated</span></div>
    <p class="lg-section-note">Every rule carries a confidence level — <span class="lg-conf lg-high" style="display:inline-flex"><span class="lg-dot"></span>high</span>, <span class="lg-conf lg-medium" style="display:inline-flex"><span class="lg-dot"></span>medium</span>, or <span class="lg-conf lg-low" style="display:inline-flex"><span class="lg-dot"></span>low</span> — so review has something concrete to look at, not a wall of unlabeled YAML.</p>
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
        <span class="lg-conf lg-medium"><span class="lg-dot"></span>needs --history</span>
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
    <p class="lg-section-note">landlock-genprof doesn't enforce anything itself — it feeds three existing, independent enforcement mechanisms, one per domain, in the format each already expects. None of them are installed by this project, and <strong>generating an artifact is not the same as enforcing it</strong>: each card below says how far v0.2.0 actually goes. What each mechanism needs: <a href="docs/enforcement-prerequisites.html">enforcement prerequisites</a>. Full positioning against PodLock/SPO/static compliance scanners: <a href="docs/product-definition-v1.html">product definition</a>. (Using SPO as an upstream evidence source, rather than only as a downstream backend, is future work — not part of v0.2.0.)</p>
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
        <span class="lg-conf lg-low"><span class="lg-dot"></span>seccomp enforcement not demonstrated in v0.2.0</span>
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
