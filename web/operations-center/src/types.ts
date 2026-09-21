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

export interface LineageDiagnostics { excludedMalformed?: number; excludedInsufficientProvenance?: number; excludedMixedIdentity?: number; excludedNotAssociated?: number }
export interface WorkloadPolicyResponse { identity: { clusterIdentity: string; namespace: string; group: string; kind: string; workloadName: string; workloadUID: string; container: string }; proposals: ProposalRead[]; complete: boolean; continue?: string; diagnostics?: LineageDiagnostics }
export interface AttemptRead { namespace: string; name: string; uid: string; resourceVersion?: string; proposalNamespace?: string; proposalName?: string; proposalUID?: string; sourceNamespace?: string; sourceName?: string; sourceUID?: string; approvedCandidateDigest?: string; target?: string; operator?: string; custodyEpoch?: string; state?: string; startedAt?: string; updatedAt?: string; completedAt?: string; failure?: unknown; mutations?: Array<Record<string, unknown>> }
export interface AttemptListResponse { items: AttemptRead[]; complete: boolean; continue?: string; diagnostics?: LineageDiagnostics }
export interface ProposalListResponse { items: ProposalRead[]; limit?: number; projectionStatus?: string; diagnostics?: unknown[]; complete?: boolean; continue?: string }
export interface ProposalGenerationResponse { proposalName: string; candidateVersion?: string; scope?: string; target?: string; container?: string; imageIdentity?: string; approved?: boolean }
export interface GovernanceResponse { operation: string; resultingState?: string; previousState?: string; actor?: string; success: boolean; message?: string; attempt?: string }

export interface AppContext {
  cluster: string;
  namespace: string;
  sessionID: string;
  contextVersion: number;
  identity: string;
}

export interface HistorySourceRef {
  kind: string;
  namespace?: string;
  name?: string;
  uid?: string;
}

export interface HistoryEvent {
  kind: string;
  sourceRef: HistorySourceRef;
  relatedRef?: HistorySourceRef;
  timestamp?: string;
  temporalClass: string;
  claimTier: string;
  detailCode: string;
}

export interface HistoryProjection {
  timestampedEvents: HistoryEvent[];
  untimestampedFacts: HistoryEvent[];
  limitations: string[];
  totalCount: number;
  truncated: boolean;
}

export interface HistoryResponse {
  history: HistoryProjection;
  limitation: string;
  projectionDiagnostics?: unknown;
}

export interface EnvironmentSubject {
  scope?: string;
  target?: string;
  container?: string;
  imageIdentity?: string;
  binaryPath?: string;
}

export interface AttentionRef {
  namespace?: string;
  name?: string;
  uid?: string;
}

export interface AttentionReason {
  category: string;
  subject?: EnvironmentSubject;
  evidenceRefs?: string[];
  observationRefs?: string[];
  proposalRefs?: AttentionRef[];
  governanceRefs?: AttentionRef[];
  applicationRefs?: AttentionRef[];
  explanationCode?: string;
}

export interface EnvironmentProjectionItem {
  subject: EnvironmentSubject;
  attention?: AttentionReason[];
}

export interface EnvironmentProjectionResponse {
  items: EnvironmentProjectionItem[];
  totalCount?: number;
  truncated?: boolean;
  unattributedFailedObservationCount?: number;
  limitation?: string;
  projectionDiagnostics?: unknown;
}

export interface SphmDimension {
  id: string;
  name: string;
  state: string;
  value?: number;
  unit?: string;
  reason?: string;
  authoritativeSource?: string;
  drilldown?: string;
  evaluatedAt?: string;
}

export interface SphmAttention {
  id: string;
  kind: string;
  title: string;
  reason: string;
  observationID?: string;
  proposalName?: string;
  workload?: string;
}

export interface SphmReport {
  modelVersion: string;
  overall: SphmDimension;
  dimensions: SphmDimension[];
  attention: SphmAttention[];
  context: { clusterIdentity: string; namespace: string; contextVersion: string };
}

export interface OverviewProjectionResponse {
  environment: EnvironmentProjectionResponse;
  history: HistoryResponse;
  limitation: string;
}
