# Security policy

## Supported versions

Security fixes are evaluated against the latest tagged release and the
current `master` branch. Users should upgrade to the latest release before
requesting support for an older tag.

## Reporting a vulnerability

Please do **not** disclose an unpatched vulnerability in a public GitHub issue
or pull request. Prefer the repository's GitHub **Security Advisories**
"Report a vulnerability" workflow when it is available. If that private
workflow is not enabled for the repository, contact the sole maintainer,
[@idriss-eliguene](https://github.com/idriss-eliguene), through the GitHub
profile and request a private channel before sending sensitive details.

Include:

* affected commit, tag, image, chart, or deployment;
* Kubernetes, kernel, runtime, and backend versions where relevant;
* a minimal reproduction that does not contain credentials or private data;
* security impact and the narrowest required permissions;
* any proposed mitigation and whether exploitation is active.

Do not include kubeconfigs, tokens, HMAC material, private manifests, customer
data, or unredacted cluster logs. Use placeholders and explain how the
maintainer can reproduce the issue safely.

The repository currently has GitHub Dependabot security updates and secret
scanning enabled. Those automated controls do not replace private reporting
of product vulnerabilities, authorization defects, or deployment exposures.

Maintainers will acknowledge a report when practicable, determine affected
versions and severity, coordinate a fix or mitigation, and publish a concise
advisory after users have a reasonable opportunity to update. No response or
timeline is guaranteed for reports sent through public channels.

## Security boundaries

Generated policy is not applied policy, and an applied Kubernetes resource is
not proof of behavioral kernel enforcement. Reports should state what was
actually observed and must preserve the project's evidence, provenance,
approval, digest, and authorization boundaries.
