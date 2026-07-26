import { z } from "zod";

export const HSM_PROTOCOL_VERSION = 1;
export const HSM_LEGAL_ACTION_COUNT = 49;

const nonNegativeInteger = z.number().int().nonnegative();
const positiveInteger = z.number().int().positive();
const nonEmptyString = z.string().trim().min(1);
const canonicalActionID = nonNegativeInteger.max(HSM_LEGAL_ACTION_COUNT - 1);
const hsmClassifier = z.enum([
  "focused-clue",
  "play-response",
  "prompt-response",
  "finesse-response",
  "good-touch",
  "save-principle",
  "mcvp",
  "early-game",
  "trash-before-chop",
  "hierarchy-resolved",
]);
const booleanVector = z.boolean().array().readonly();
const legalActionVector = z
  .boolean()
  .array()
  .length(HSM_LEGAL_ACTION_COUNT)
  .readonly();

export const hsmResponseIdentity = z
  .object({
    protocolVersion: z.literal(HSM_PROTOCOL_VERSION),
    serverRequestID: positiveInteger,
    clientRequestID: positiveInteger,
    archiveGenerationID: positiveInteger,
    targetBoundary: nonNegativeInteger,
    evidenceBoundary: nonNegativeInteger,
    perspectivePlayer: nonNegativeInteger,
    actorPlayer: z.number().int().min(-1),
    semanticProfileID: positiveInteger,
    authorityLegalProjectionDigest: z.string().regex(/^sha256:[0-9a-f]{64}$/),
  })
  .strict()
  .readonly();

export type HSMResponseIdentity = z.infer<typeof hsmResponseIdentity>;

const hsmBeliefReason = z
  .object({
    reason: nonEmptyString,
    identity_mask: positiveInteger,
  })
  .strict()
  .readonly();

const hsmCardBelief = z
  .object({
    stable_card_id: nonNegativeInteger,
    candidate_identity_mask: positiveInteger,
    reason_identity_masks: hsmBeliefReason.array().readonly(),
  })
  .strict()
  .readonly();

const hsmConventionApplication = z
  .object({
    source_transition: nonNegativeInteger,
    historical_actor: nonNegativeInteger,
    outer_observer: nonNegativeInteger,
    rule: nonEmptyString,
    meaning: nonEmptyString,
    subject_kind: nonEmptyString,
    subject_id: nonNegativeInteger,
    provenance_id: nonNegativeInteger,
    applicable: z.boolean(),
  })
  .strict()
  .readonly();

export const hsmClassification = z
  .object({
    action_id: canonicalActionID,
    classifier: hsmClassifier,
    follow: z.boolean(),
    violation: z.boolean(),
  })
  .strict()
  .readonly();

const hsmSemanticValue = z
  .object({
    action_id: canonicalActionID,
    category: nonEmptyString,
    name: nonEmptyString,
    active: z.boolean(),
  })
  .strict()
  .readonly();

const hsmConnectionCard = z
  .object({
    stable_card_id: nonNegativeInteger,
    identity_mask: positiveInteger,
  })
  .strict()
  .readonly();

const hsmPlayConnection = z
  .object({
    source_transition: nonNegativeInteger,
    available_from_boundary: nonNegativeInteger,
    provenance_id: nonNegativeInteger,
    focus_card_id: nonNegativeInteger,
    focus_identity_mask: positiveInteger,
    prerequisites: hsmConnectionCard.array().readonly(),
  })
  .strict()
  .readonly();

const hsmConnectionObligation = z
  .object({
    source_transition: nonNegativeInteger,
    available_from_boundary: nonNegativeInteger,
    provenance_id: nonNegativeInteger,
    kind: z.enum(["prompt", "finesse"]),
    owner_player: nonNegativeInteger,
    focus_card_id: nonNegativeInteger,
    focus_identity_mask: positiveInteger,
    current_candidate_index: nonNegativeInteger,
    candidates: hsmConnectionCard.array().min(1).readonly(),
  })
  .strict()
  .readonly()
  .refine(
    (obligation) =>
      obligation.current_candidate_index < obligation.candidates.length,
    "current_candidate_index must select an existing candidate",
  );

const hsmDiagnosticProjection = z
  .object({
    applications: hsmConventionApplication.array().readonly(),
    card_beliefs: hsmCardBelief.array().readonly(),
    play_connections: hsmPlayConnection.array().readonly(),
    connection_obligations: hsmConnectionObligation.array().readonly(),
    classifications: hsmClassification.array().readonly(),
    semantic_values: hsmSemanticValue.array().readonly(),
  })
  .strict()
  .readonly();

const hsmPhysicalGuard = z
  .object({
    clause_ids: z
      .string()
      .regex(/^sha256:[0-9a-f]{64}$/)
      .array()
      .min(1)
      .readonly(),
    evidence_boundary: nonNegativeInteger,
    semantic_profile_id: positiveInteger,
  })
  .strict()
  .readonly();

const hsmDiagnosis = hsmDiagnosticProjection
  .unwrap()
  .extend({
    label: z.string().regex(/^hsm-diagnosis:[0-9a-f]{64}(?::\d+)?$/),
    physical_guard: hsmPhysicalGuard,
  })
  .strict()
  .readonly();

const hsmViolationWarning = z
  .object({
    action_id: canonicalActionID,
    diagnosis_count: z.number().int().positive(),
    total_diagnoses: z.number().int().positive(),
  })
  .strict()
  .readonly()
  .refine(
    (warning) => warning.diagnosis_count <= warning.total_diagnoses,
    "diagnosis_count cannot exceed total_diagnoses",
  );

const hsmMistakenAction = z
  .object({
    transition_index: nonNegativeInteger,
    historical_actor: nonNegativeInteger,
    action_id: canonicalActionID,
    violating_classifiers: hsmClassifier.array().min(1).readonly(),
  })
  .strict()
  .readonly();

const hsmActionTimeClassification = z
  .object({
    generation_id: positiveInteger,
    target_boundary: nonNegativeInteger,
    evidence_boundary: nonNegativeInteger,
    perspective_player: nonNegativeInteger,
    semantic_profile_id: positiveInteger,
    selected_action_id: canonicalActionID,
    final_follow: z.boolean(),
    final_violation: z.boolean(),
    rule_follow: booleanVector,
    rule_violation: booleanVector,
  })
  .strict()
  .readonly()
  .refine(
    (record) => record.evidence_boundary === record.target_boundary,
    "action-time evidence must equal its target boundary",
  )
  .refine(
    (record) => record.rule_follow.length === record.rule_violation.length,
    "action-time classifier vectors must have equal length",
  );

export const hsmSnapshot = z
  .object({
    generation_id: positiveInteger,
    target_boundary: nonNegativeInteger,
    evidence_boundary: nonNegativeInteger,
    perspective_player: nonNegativeInteger,
    semantic_program_id: z.string().regex(/^sha256:[0-9a-f]{64}$/),
    semantic_profile_id: positiveInteger,
    aggregate_action_classifications: hsmClassification.array().readonly(),
    mistaken_actions: hsmMistakenAction.array().readonly(),
    diagnoses: hsmDiagnosis.array().min(1).readonly(),
    consensus: hsmDiagnosticProjection,
  })
  .strict()
  .readonly()
  .superRefine((snapshot, context) => {
    if (
      snapshot.consensus.classifications.some(
        (classification) => !classification.follow && !classification.violation,
      )
    ) {
      context.addIssue({
        code: "custom",
        message: "consensus must omit non-universal classifications",
        path: ["consensus", "classifications"],
      });
    }
    if (
      new Set(snapshot.diagnoses.map((diagnosis) => diagnosis.label)).size
      !== snapshot.diagnoses.length
    ) {
      context.addIssue({
        code: "custom",
        message: "diagnosis labels must be unique",
        path: ["diagnoses"],
      });
    }
    const clauseIDs = snapshot.diagnoses.flatMap(
      (diagnosis) => diagnosis.physical_guard.clause_ids,
    );
    if (new Set(clauseIDs).size !== clauseIDs.length) {
      context.addIssue({
        code: "custom",
        message: "physical guard clause_ids must be globally unique",
        path: ["diagnoses"],
      });
    }
    for (const [index, diagnosis] of snapshot.diagnoses.entries()) {
      if (
        diagnosis.physical_guard.evidence_boundary
          !== snapshot.evidence_boundary
        || diagnosis.physical_guard.semantic_profile_id
          !== snapshot.semantic_profile_id
      ) {
        context.addIssue({
          code: "custom",
          message: "physical guard coordinate must match the snapshot",
          path: ["diagnoses", index, "physical_guard"],
        });
      }
    }
  });

export type HSMSnapshot = z.infer<typeof hsmSnapshot>;
export type HSMDiagnosticProjection = z.infer<typeof hsmDiagnosticProjection>;
export type HSMDiagnosis = z.infer<typeof hsmDiagnosis>;
export type HSMClassification = z.infer<typeof hsmClassification>;
export type HSMBeliefReason = z.infer<typeof hsmBeliefReason>;
export type HSMConventionApplication = z.infer<typeof hsmConventionApplication>;
export type HSMCardBelief = z.infer<typeof hsmCardBelief>;
export type HSMPlayConnection = z.infer<typeof hsmPlayConnection>;
export type HSMConnectionObligation = z.infer<typeof hsmConnectionObligation>;
export type HSMConnectionCard = z.infer<typeof hsmConnectionCard>;
export type HSMPhysicalGuard = z.infer<typeof hsmPhysicalGuard>;
export type HSMSemanticValue = z.infer<typeof hsmSemanticValue>;
export type HSMMistakenAction = z.infer<typeof hsmMistakenAction>;
export type HSMActionTimeClassification = z.infer<
  typeof hsmActionTimeClassification
>;
export type HSMViolationWarning = z.infer<typeof hsmViolationWarning>;

const hsmUnsatisfiableCore = z
  .object({
    subset_minimal: z.boolean(),
    coordinate_kind: z
      .enum([
        "evaluation",
        "stable_card",
        "transition",
        "rule",
        "action",
        "evidence",
      ])
      .array()
      .min(1)
      .readonly(),
    transition_index: z.number().int().array().readonly(),
    rule_index: z.number().int().array().readonly(),
    subject_index: z.number().int().array().readonly(),
    evidence_boundary: z.number().int().array().readonly(),
    provenance_id: z.number().int().array().readonly(),
  })
  .strict()
  .readonly()
  .superRefine((core, context) => {
    const length = core.coordinate_kind.length;
    if (
      core.transition_index.length !== length
      || core.rule_index.length !== length
      || core.subject_index.length !== length
      || core.evidence_boundary.length !== length
      || core.provenance_id.length !== length
    ) {
      context.addIssue({
        code: "custom",
        message: "trimmed unsatisfiable core arrays must share one shape",
      });
    }
  });

const hsmInvariantFailure = z
  .object({
    primary_defect: nonEmptyString,
    defect: nonEmptyString.array().min(1).readonly(),
    transition_index: z.number().int().array().readonly(),
    rule_index: z.number().int().array().readonly(),
    subject_index: z.number().int().array().readonly(),
    evidence_boundary: z.number().int().array().readonly(),
    provenance_id: z.number().int().array().readonly(),
  })
  .strict()
  .readonly()
  .superRefine((failure, context) => {
    const length = failure.defect.length;
    if (
      failure.transition_index.length !== length
      || failure.rule_index.length !== length
      || failure.subject_index.length !== length
      || failure.evidence_boundary.length !== length
      || failure.provenance_id.length !== length
    ) {
      context.addIssue({
        code: "custom",
        message: "trimmed invariant failure arrays must share one shape",
      });
    }
  });

const hsmFailureCategory = z.enum([
  "observer_evidence_unsatisfiable",
  "semantic_program_unsatisfiable",
  "structural_invariant_failure",
]);
const hsmFailurePhase = z.enum([
  "archive_projection",
  "semantic_compilation",
  "exact_solving",
  "result_projection",
  "structural_validation",
]);

export const hsmFailure = z
  .object({
    category: hsmFailureCategory,
    phase: hsmFailurePhase,
    topology_id: nonNegativeInteger,
    capacity_manifest_id: z.string().regex(/^sha256:[0-9a-f]{64}$/),
    semantic_program_id: z.string().regex(/^sha256:[0-9a-f]{64}$/),
    semantic_profile_id: positiveInteger,
    legal_action_projection: legalActionVector,
    unsatisfiable_core: hsmUnsatisfiableCore.optional(),
    invariant_failure: hsmInvariantFailure.optional(),
  })
  .strict()
  .readonly()
  .superRefine((failure, context) => {
    const hasCore = failure.unsatisfiable_core !== undefined;
    const hasInvariant = failure.invariant_failure !== undefined;
    if (hasCore && hasInvariant) {
      context.addIssue({
        code: "custom",
        message: "failure causes are mutually exclusive",
      });
    }
    const isLogical =
      failure.category === "observer_evidence_unsatisfiable"
      || failure.category === "semantic_program_unsatisfiable";
    const isStructural = failure.category === "structural_invariant_failure";
    if (isLogical !== hasCore || isStructural !== hasInvariant) {
      context.addIssue({
        code: "custom",
        message: "failure category must own exactly its typed cause",
      });
    }
  });

export type HSMFailure = z.infer<typeof hsmFailure>;
export type HSMUnsatisfiableCore = z.infer<typeof hsmUnsatisfiableCore>;
export type HSMInvariantFailure = z.infer<typeof hsmInvariantFailure>;

export const hsmSnapshotMessage = hsmResponseIdentity
  .unwrap()
  .extend({
    snapshot: hsmSnapshot,
  })
  .strict()
  .readonly()
  .superRefine((message, context) => {
    if (
      message.snapshot.generation_id !== message.archiveGenerationID
      || message.snapshot.target_boundary !== message.targetBoundary
      || message.snapshot.evidence_boundary !== message.evidenceBoundary
      || message.snapshot.perspective_player !== message.perspectivePlayer
      || message.snapshot.semantic_profile_id !== message.semanticProfileID
    ) {
      context.addIssue({
        code: "custom",
        message: "snapshot coordinate must match response identity",
        path: ["snapshot"],
      });
    }
  });

export const hsmSnapshotFailureMessage = hsmResponseIdentity
  .unwrap()
  .extend({
    error: nonEmptyString,
    failure: hsmFailure,
  })
  .strict()
  .readonly()
  .superRefine((message, context) => {
    if (message.failure.semantic_profile_id !== message.semanticProfileID) {
      context.addIssue({
        code: "custom",
        message: "failure semantic profile must match response identity",
        path: ["failure", "semantic_profile_id"],
      });
    }
  });

export const HSM_SNAPSHOT_UNAVAILABLE_REASON = "diagnostics_unavailable";
export const HSM_SNAPSHOT_UNAVAILABLE_ERROR =
  "HSM diagnostics are unavailable.";

export const hsmSnapshotUnavailableMessage = hsmResponseIdentity
  .unwrap()
  .extend({
    reasonCode: z.literal(HSM_SNAPSHOT_UNAVAILABLE_REASON),
    error: z.literal(HSM_SNAPSHOT_UNAVAILABLE_ERROR),
  })
  .strict()
  .readonly();

export const hsmSnapshotRejectedMessage = z
  .object({
    protocolVersion: z.literal(HSM_PROTOCOL_VERSION),
    clientRequestID: nonNegativeInteger,
    archiveGenerationID: positiveInteger,
    targetBoundary: nonNegativeInteger,
    evidenceBoundary: nonNegativeInteger,
    perspectivePlayer: nonNegativeInteger,
    reasonCode: nonEmptyString,
  })
  .strict()
  .readonly();

export const hsmPhysicalTruthIdentity = z
  .object({
    protocolVersion: z.literal(HSM_PROTOCOL_VERSION),
    serverRequestID: positiveInteger,
    clientRequestID: positiveInteger,
    archiveGenerationID: positiveInteger,
    targetBoundary: nonNegativeInteger,
    perspectivePlayer: nonNegativeInteger,
  })
  .strict()
  .readonly();

export const hsmPhysicalTruthOverlay = z
  .object({
    cards: z
      .object({
        stableCardID: nonNegativeInteger,
        identity: nonNegativeInteger.max(24),
      })
      .strict()
      .readonly()
      .array()
      .min(1)
      .readonly(),
  })
  .strict()
  .readonly();

export const hsmPhysicalTruthMessage = hsmPhysicalTruthIdentity
  .unwrap()
  .extend({
    overlay: hsmPhysicalTruthOverlay,
  })
  .strict()
  .readonly();

export const hsmPhysicalTruthFailureMessage = hsmPhysicalTruthIdentity
  .unwrap()
  .extend({
    error: nonEmptyString,
  })
  .strict()
  .readonly();

export const hsmPhysicalTruthRejectedMessage = z
  .object({
    protocolVersion: z.literal(HSM_PROTOCOL_VERSION),
    clientRequestID: nonNegativeInteger,
    archiveGenerationID: positiveInteger,
    targetBoundary: nonNegativeInteger,
    perspectivePlayer: nonNegativeInteger,
    reasonCode: nonEmptyString,
  })
  .strict()
  .readonly();

export type HSMSnapshotPendingMessage = HSMResponseIdentity;
export type HSMSnapshotMessage = z.infer<typeof hsmSnapshotMessage>;
export type HSMSnapshotFailureMessage = z.infer<
  typeof hsmSnapshotFailureMessage
>;
export type HSMSnapshotUnavailableMessage = z.infer<
  typeof hsmSnapshotUnavailableMessage
>;
export type HSMSnapshotRejectedMessage = z.infer<
  typeof hsmSnapshotRejectedMessage
>;
export type HSMPhysicalTruthPendingMessage = z.infer<
  typeof hsmPhysicalTruthIdentity
>;
export type HSMPhysicalTruthIdentity = z.infer<typeof hsmPhysicalTruthIdentity>;
export type HSMPhysicalTruthOverlay = z.infer<typeof hsmPhysicalTruthOverlay>;
export type HSMPhysicalTruthMessage = z.infer<typeof hsmPhysicalTruthMessage>;
export type HSMPhysicalTruthFailureMessage = z.infer<
  typeof hsmPhysicalTruthFailureMessage
>;
export type HSMPhysicalTruthRejectedMessage = z.infer<
  typeof hsmPhysicalTruthRejectedMessage
>;

export const hsmSnapshotRequestCommand = z
  .object({
    tableID: nonNegativeInteger,
    protocolVersion: nonNegativeInteger,
    archiveGenerationID: positiveInteger,
    clientRequestID: nonNegativeInteger,
    targetBoundary: nonNegativeInteger,
    evidenceBoundary: nonNegativeInteger,
    perspectivePlayer: nonNegativeInteger,
  })
  .strict()
  .readonly();

export const hsmPhysicalTruthRequestCommand = z
  .object({
    tableID: nonNegativeInteger,
    protocolVersion: nonNegativeInteger,
    archiveGenerationID: positiveInteger,
    clientRequestID: nonNegativeInteger,
    targetBoundary: nonNegativeInteger,
    perspectivePlayer: nonNegativeInteger,
  })
  .strict()
  .readonly();

export type HSMSnapshotRequestCommand = z.infer<
  typeof hsmSnapshotRequestCommand
>;
export type HSMPhysicalTruthRequestCommand = z.infer<
  typeof hsmPhysicalTruthRequestCommand
>;

export const hsmTransportGolden = z
  .object({
    protocolVersion: z.literal(HSM_PROTOCOL_VERSION),
    snapshotPending: hsmResponseIdentity,
    snapshotMessage: hsmSnapshotMessage,
    snapshotFailure: hsmSnapshotFailureMessage,
    snapshotRejected: hsmSnapshotRejectedMessage,
    physicalTruthPending: hsmPhysicalTruthIdentity,
    physicalTruthMessage: hsmPhysicalTruthMessage,
    physicalTruthFailure: hsmPhysicalTruthFailureMessage,
    physicalTruthRejected: hsmPhysicalTruthRejectedMessage,
  })
  .strict()
  .readonly();

export type HSMTransportGolden = z.infer<typeof hsmTransportGolden>;
