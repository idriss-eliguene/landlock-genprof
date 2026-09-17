# Commercial demo

## Five-minute flow

1. Run `make operations-center-demo` and open the printed URL.
2. Point out Cluster, Identity, Namespace, and Healthy connection state.
3. Open Workloads and inspect `frontend` or `api`.
4. Start observation; wait until the UI/executor reports the active capture
   window, then trigger the deterministic workload activity.
5. Open the completed observation. Show capability facts and Evidence
   captured, then generate and inspect the proposal.
6. Open History, Attention, and Governance. Perform the authorized action;
   demonstrate the denied action with the restricted identity.
7. Switch identity/namespace and show capabilities and data rebind safely.

Reset with `make operations-center-demo-reset`. If the local runtime is
unavailable, rerun the canonical command after checking Lima/Docker status;
do not manually create credentials or Kubernetes resources.
