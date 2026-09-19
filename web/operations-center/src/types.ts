export interface ContextRecord {
  clusterIdentity: string;
  clusterDisplayName?: string;
  clusterName?: string;
  contextName: string;
  defaultNamespace?: string;
}

export interface EnvironmentSession {
  sessionID: string;
  contextName: string;
  defaultNamespace?: string;
}

export interface NamespaceDiscovery {
  mode: "DISCOVERED" | "EXPLICIT_ONLY" | string;
  namespaces?: string[];
}

export interface BoundCapabilities {
  namespace: string;
  contextVersion: number;
  capabilities?: Record<string, boolean>;
}

export interface OperationalContext {
  readTime: string;
  context: {
    cluster: { identity: string; context: string; status: string };
    namespace: string;
    actor: { username: string };
    credential: { source?: string; authMethod?: string };
  };
  authority: { status: string; capabilities?: Record<string, boolean> };
  platform: { status: string; backend: string; kubernetesAPI: string };
}

export interface WorkloadContainer {
  name: string;
  supportedTarget?: boolean;
  runtime?: { imageID?: string };
  target?: { workload?: { group: string; kind: string; name: string } };
}

export interface WorkloadPod {
  name: string;
  uid?: string;
  containers?: WorkloadContainer[];
}

export interface WorkloadRecord {
  uid?: string;
  target?: { group: string; kind: string; name: string };
  pods?: WorkloadPod[];
}

export interface WorkloadResponse {
  namespace: string;
  workloads: WorkloadRecord[];
}

export interface WorkloadSelection {
  group: string;
  kind: string;
  name: string;
  container: string;
  workloadUID: string;
  imageIdentity?: string;
}

export interface WorkloadDetail {
  workload: Record<string, unknown>;
  yaml: string;
  source: string;
}

export interface AppContext {
  cluster: string;
  namespace: string;
  sessionID: string;
  contextVersion: number;
  identity: string;
}
