# Restricted user scenario

Namespace LIST is denied by RBAC. The UI enters explicit-only mode and says
discovery is restricted. A known authorized namespace succeeds; an unrelated
namespace is denied. No fake namespaces or broader RBAC are introduced.
