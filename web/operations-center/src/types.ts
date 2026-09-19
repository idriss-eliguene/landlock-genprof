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
  pod?: string;
  workloadUID: string;
  imageIdentity?: string;
}

export interface WorkloadDetail {
  workload: Record<string, unknown>;
  yaml: string;
  source: string;
}

export interface ObservationExecution {
  state?: string;
  completion?: string;
  startedAt?: string;
  completedAt?: string;
  stopRequestedAt?: string;
  failure?: {
    stage?: string;
    code?: string;
    reason?: string;
    source?: string;
    occurredAt?: string;
    retryable?: boolean;
    executorID?: string;
    claimGeneration?: number;
  };
}

export interface ObservationSource {
  name: string;
  attributionState: string;
  evidenceState: string;
  attributedCount: number;
  excludedCount: number;
  backendHealthConfirmed: boolean;
  sourceAttachedForBoundWindow: boolean;
  flushConfirmed: boolean;
  facts?: unknown;
  references?: string[];
}

export interface ObservationRead {
  observationID: string;
  identity: {
    clusterIdentity: string;
    namespace: string;
    group: string;
    kind: string;
    workloadName: string;
    workloadUID: string;
    container: string;
    imageIdentity?: string;
  };
  spec?: { sources?: string[]; duration?: string; requesterSession?: string };
  execution: ObservationExecution;
  sources: ObservationSource[];
  frozen: boolean;
  stopEligible: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface ObservationListResponse {
  items: ObservationRead[];
  projectionStatus?: string;
  diagnostics?: Array<{ category?: string; reason?: string; name?: string }>;
}

export interface ObservationStartResponse {
  observationID: string;
  executionState: string;
  frozen: boolean;
  stopEligible: boolean;
}

export interface ObservationStopResponse extends ObservationStartResponse {
  stopRequested: boolean;
}

export interface ProposalStatus {
  approvalState?: string;
  reviewedBy?: string;
  approvedBy?: string;
  rejectedBy?: string;
  reason?: string;
  updatedAt?: string;
  approvedCandidateDigest?: string;
  approvedReviewContextDigest?: string;
  approvalMechanismVersion?: string;
}

export interface ProposalRead {
  name: string;
  uid?: string;
  resourceVersion?: string;
  candidateVersion: string;
  subject?: { scope?: string; target?: string; container?: string; imageIdentity?: string };
  artifact?: { type?: string; containerCapabilities?: { drop?: string[]; add?: string[] } };
  candidateDigest?: string;
  reviewContextDigest?: string;
  provenance?: { populationScope?: string; observationIDs?: string[] };
  qualification?: Record<string, string>;
  derivationStatus?: Record<string, string>;
  candidateYAML?: string;
  candidateJSON?: string;
  status: ProposalStatus;
  currentAuthority?: string;
  creationTimestamp?: string;
}

export interface ProposalListResponse { items: ProposalRead[]; limit?: number; projectionStatus?: string; diagnostics?: unknown[] }
export interface ProposalGenerationResponse { proposalName: string; candidateVersion?: string; scope?: string; target?: string; container?: string; imageIdentity?: string; approved?: boolean }
export interface GovernanceResponse { operation: string; resultingState?: string; previousState?: string; actor?: string; success: boolean; message?: string; attempt?: string }

export interface AppContext {
  cluster: string;
  namespace: string;
  sessionID: string;
  contextVersion: number;
  identity: string;
}
