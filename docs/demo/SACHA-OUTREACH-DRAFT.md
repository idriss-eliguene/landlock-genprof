# Unsent technical outreach draft — Sacha

Subject: A narrowly scoped Seccomp verification experiment in landlock-genprof

Hi Sacha,

I’m working on an opt-in, bounded Seccomp verification path for
landlock-genprof. It compares one fixed `getpriority` probe in a short-lived
Pod using an approved SPO Localhost profile with the same probe in a
`RuntimeDefault` control Pod on the same node and runtime. The operation is
separately authorized, records per-Pod identities and outcomes, and keeps
profile materialization, workload configuration, and probe behavior as
separate evidence.

The deliberately narrow claim is only about that probe in the twin Pod; it
does not establish that the application process has the filter active or
prove general Seccomp correctness. The implementation is still being
validated, so I’m not claiming successful runtime qualification yet. If the
approach seems useful, I’d value your feedback on the evidence boundary and
operational fit before presenting it as a qualified capability.

Best,
Idriss
