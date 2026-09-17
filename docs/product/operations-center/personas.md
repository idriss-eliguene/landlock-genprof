# Personas

## Developer

Wants to inspect permitted workloads, run authorized observations, and
understand resulting evidence. Namespace and mutation access are determined
by the selected Kubernetes credential, not by this label.

## Security Reviewer

Reviews evidence, provenance, proposals, history, and governance decisions.
May have governance capability where Kubernetes RBAC permits it.

## Restricted User

Works in a known namespace but cannot list namespaces. The product explains
restricted discovery and permits explicit namespace validation when the
identity is authorized.

Persona != Kubernetes identity != credential != authorization. Personas are
fixture/story labels only; runtime authority is Kubernetes RBAC/SSAR.
