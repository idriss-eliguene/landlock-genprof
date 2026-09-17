# Authorization UX matrix

The table describes demo expectations, not production policy.

| Capability | Developer | Security Reviewer | Restricted | UX |
|---|---|---|---|---|
| List namespaces | SSAR | SSAR | denied example | selector or explicit input |
| Read workloads | SSAR | SSAR | namespace-scoped SSAR | show usable workloads |
| Start observation | SSAR + workflow | SSAR + workflow | bounded namespace | enable only when authorized |
| Generate proposal | evidence + SSAR | evidence + SSAR | evidence + SSAR | explain unavailable |
| Governance | SSAR | SSAR | denied example | deliberate action/denial |

The backend remains authoritative even when controls are hidden or disabled.
