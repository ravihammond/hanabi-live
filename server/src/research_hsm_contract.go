package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var researchHSMDiagnosisLabel = regexp.MustCompile(
	`^hsm-diagnosis:[0-9a-f]{64}(?::[0-9]+)?$`,
)

var researchHSMSemanticProgramID = regexp.MustCompile(
	`^sha256:[0-9a-f]{64}$`,
)

type ResearchHSMCapability string

type ResearchHSMViewerKind string

const (
	ResearchHSMProtocolVersion                                     = 1
	researchHSMLegalProjectionSize                                 = 49
	researchHSMLegalProjectionEncoding                             = "canonical-action-legality-v1"
	researchHSMSnapshotUnavailableReasonCode                       = "diagnostics_unavailable"
	researchHSMSnapshotUnavailableError                            = "HSM diagnostics are unavailable."
	ResearchHSMCapabilityNone                ResearchHSMCapability = "none"
	ResearchHSMCapabilityOwnPerspective      ResearchHSMCapability = "own_perspective"
	ResearchHSMCapabilitySwitchable          ResearchHSMCapability = "switchable"

	ResearchHSMViewerKindParticipant ResearchHSMViewerKind = "participant"
	ResearchHSMViewerKindSpectator   ResearchHSMViewerKind = "spectator"
)

type ResearchHSMLegalActionLegality struct {
	CanonicalActionID int  `json:"canonical_action_id"`
	Legal             bool `json:"legal"`
}

// ResearchHSMLegalProjection binds one authority-owned legal projection to a
// stable, map-order-independent digest.
type ResearchHSMLegalProjection struct {
	Encoding string                           `json:"encoding"`
	Actions  []ResearchHSMLegalActionLegality `json:"actions"`
	Digest   string                           `json:"digest"`
}

func newResearchHSMLegalProjection(legality []bool) ResearchHSMLegalProjection {
	actions := make([]ResearchHSMLegalActionLegality, researchHSMLegalProjectionSize)
	for actionID := range actions {
		actions[actionID] = ResearchHSMLegalActionLegality{
			CanonicalActionID: actionID,
			Legal:             actionID < len(legality) && legality[actionID],
		}
	}
	projection := ResearchHSMLegalProjection{
		Encoding: researchHSMLegalProjectionEncoding,
		Actions:  actions,
	}
	projection.Digest = projection.expectedDigest()
	return projection
}

func (projection ResearchHSMLegalProjection) expectedDigest() string {
	var encoded strings.Builder
	encoded.WriteString(researchHSMLegalProjectionEncoding)
	encoded.WriteByte('\n')
	for _, action := range projection.Actions {
		encoded.WriteString(strconv.Itoa(action.CanonicalActionID))
		encoded.WriteByte('=')
		if action.Legal {
			encoded.WriteByte('1')
		} else {
			encoded.WriteByte('0')
		}
		encoded.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(encoded.String()))
	return fmt.Sprintf("sha256:%x", sum)
}

func (projection ResearchHSMLegalProjection) validate() error {
	if projection.Encoding != researchHSMLegalProjectionEncoding {
		return fmt.Errorf("HSM legal projection encoding is not supported")
	}
	if len(projection.Actions) != researchHSMLegalProjectionSize {
		return fmt.Errorf(
			"HSM legal projection requires %d fixed Canonical Action IDs",
			researchHSMLegalProjectionSize,
		)
	}
	for actionID, action := range projection.Actions {
		if action.CanonicalActionID != actionID {
			return fmt.Errorf("HSM legal projection Canonical Action IDs must be sorted and complete")
		}
	}
	if projection.Digest == "" || projection.Digest != projection.expectedDigest() {
		return fmt.Errorf("HSM legal projection digest does not match its exact projection")
	}
	return nil
}

func (projection ResearchHSMLegalProjection) legality() []bool {
	legality := make([]bool, len(projection.Actions))
	for index, action := range projection.Actions {
		legality[index] = action.Legal
	}
	return legality
}

func (capability ResearchHSMCapability) valid() bool {
	switch capability {
	case "", ResearchHSMCapabilityNone,
		ResearchHSMCapabilityOwnPerspective,
		ResearchHSMCapabilitySwitchable:
		return true
	default:
		return false
	}
}

type ResearchHSMViewerAuthorization struct {
	GameID             string
	TableID            uint64
	PrincipalID        string
	Identity           string
	ViewerKind         ResearchHSMViewerKind
	Capability         ResearchHSMCapability
	PhysicalTruthGrant bool
	SeatIndex          int
}

type ResearchHSMRequestRejection struct {
	ProtocolVersion     int    `json:"protocolVersion"`
	ClientRequestID     int    `json:"clientRequestID"`
	ArchiveGenerationID uint32 `json:"archiveGenerationID"`
	TargetBoundary      int    `json:"targetBoundary"`
	EvidenceBoundary    int    `json:"evidenceBoundary"`
	PerspectivePlayer   int    `json:"perspectivePlayer"`
	ReasonCode          string `json:"reasonCode"`
}

type ResearchHSMPhysicalTruthRejection struct {
	ProtocolVersion     int    `json:"protocolVersion"`
	ClientRequestID     int    `json:"clientRequestID"`
	ArchiveGenerationID uint32 `json:"archiveGenerationID"`
	TargetBoundary      int    `json:"targetBoundary"`
	PerspectivePlayer   int    `json:"perspectivePlayer"`
	ReasonCode          string `json:"reasonCode"`
}

func (authorization ResearchHSMViewerAuthorization) allowsSnapshot(
	perspectivePlayer int,
) bool {
	switch authorization.Capability {
	case ResearchHSMCapabilityOwnPerspective:
		return perspectivePlayer == authorization.SeatIndex
	case ResearchHSMCapabilitySwitchable:
		return true
	default:
		return false
	}
}

// ResearchHSMResponseIdentity correlates one browser request, one server queue
// entry, and one exact observer-relative evaluation coordinate.
type ResearchHSMResponseIdentity struct {
	ProtocolVersion                int    `json:"protocol_version"`
	ServerRequestID                int    `json:"server_request_id"`
	ClientRequestID                int    `json:"client_request_id"`
	ArchiveGenerationID            uint32 `json:"archive_generation_id"`
	TargetBoundary                 int    `json:"target_boundary"`
	EvidenceBoundary               int    `json:"evidence_boundary"`
	PerspectivePlayer              int    `json:"perspective_player"`
	ActorPlayer                    int    `json:"actor_player"`
	SemanticProfileID              int    `json:"semantic_profile_id"`
	AuthorityLegalProjectionDigest string `json:"authority_legal_projection_digest"`
}

type ResearchHSMBeliefReason struct {
	Reason       string `json:"reason"`
	IdentityMask int    `json:"identity_mask"`
}

type ResearchHSMCardBelief struct {
	StableCardID          int                       `json:"stable_card_id"`
	CandidateIdentityMask int                       `json:"candidate_identity_mask"`
	ReasonIdentityMasks   []ResearchHSMBeliefReason `json:"reason_identity_masks"`
}

type ResearchHSMConventionApplication struct {
	SourceTransition int    `json:"source_transition"`
	HistoricalActor  int    `json:"historical_actor"`
	OuterObserver    int    `json:"outer_observer"`
	Rule             string `json:"rule"`
	Meaning          string `json:"meaning"`
	SubjectKind      string `json:"subject_kind"`
	SubjectID        int    `json:"subject_id"`
	ProvenanceID     int    `json:"provenance_id"`
	Applicable       bool   `json:"applicable"`
}

type ResearchHSMClassification struct {
	ActionID   int    `json:"action_id"`
	Classifier string `json:"classifier"`
	Follow     bool   `json:"follow"`
	Violation  bool   `json:"violation"`
}

type ResearchHSMSemanticValue struct {
	ActionID int    `json:"action_id"`
	Category string `json:"category"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
}

type ResearchHSMConnectionCard struct {
	StableCardID int `json:"stable_card_id"`
	IdentityMask int `json:"identity_mask"`
}

type ResearchHSMPlayConnection struct {
	SourceTransition      int                         `json:"source_transition"`
	AvailableFromBoundary int                         `json:"available_from_boundary"`
	ProvenanceID          int                         `json:"provenance_id"`
	FocusCardID           int                         `json:"focus_card_id"`
	FocusIdentityMask     int                         `json:"focus_identity_mask"`
	Prerequisites         []ResearchHSMConnectionCard `json:"prerequisites"`
}

type ResearchHSMConnectionObligation struct {
	SourceTransition      int                         `json:"source_transition"`
	AvailableFromBoundary int                         `json:"available_from_boundary"`
	ProvenanceID          int                         `json:"provenance_id"`
	Kind                  string                      `json:"kind"`
	OwnerPlayer           int                         `json:"owner_player"`
	FocusCardID           int                         `json:"focus_card_id"`
	FocusIdentityMask     int                         `json:"focus_identity_mask"`
	CurrentCandidateIndex int                         `json:"current_candidate_index"`
	Candidates            []ResearchHSMConnectionCard `json:"candidates"`
}

type ResearchHSMDiagnosticProjection struct {
	Applications          []ResearchHSMConventionApplication `json:"applications"`
	CardBeliefs           []ResearchHSMCardBelief            `json:"card_beliefs"`
	PlayConnections       []ResearchHSMPlayConnection        `json:"play_connections"`
	ConnectionObligations []ResearchHSMConnectionObligation  `json:"connection_obligations"`
	Classifications       []ResearchHSMClassification        `json:"classifications"`
	SemanticValues        []ResearchHSMSemanticValue         `json:"semantic_values"`
}

func (projection ResearchHSMDiagnosticProjection) validate(
	name string,
	consensus bool,
) error {
	if projection.Applications == nil ||
		projection.CardBeliefs == nil ||
		projection.PlayConnections == nil ||
		projection.ConnectionObligations == nil ||
		projection.Classifications == nil ||
		projection.SemanticValues == nil {
		return fmt.Errorf("%s must publish every required diagnostic collection", name)
	}
	for _, application := range projection.Applications {
		if application.SourceTransition < 0 ||
			application.HistoricalActor < 0 ||
			application.OuterObserver < 0 ||
			strings.TrimSpace(application.Rule) == "" ||
			strings.TrimSpace(application.Meaning) == "" ||
			strings.TrimSpace(application.SubjectKind) == "" ||
			application.SubjectID < 0 ||
			application.ProvenanceID < 0 {
			return fmt.Errorf("%s contains an incomplete convention application", name)
		}
	}
	cardIDs := make(map[int]bool, len(projection.CardBeliefs))
	for _, belief := range projection.CardBeliefs {
		if belief.StableCardID < 0 ||
			belief.CandidateIdentityMask <= 0 ||
			belief.ReasonIdentityMasks == nil {
			return fmt.Errorf("%s contains an incomplete observer-relative card belief", name)
		}
		if cardIDs[belief.StableCardID] {
			return fmt.Errorf("%s duplicates Stable Card ID %d", name, belief.StableCardID)
		}
		cardIDs[belief.StableCardID] = true
		for _, reason := range belief.ReasonIdentityMasks {
			if strings.TrimSpace(reason.Reason) == "" || reason.IdentityMask <= 0 {
				return fmt.Errorf("%s contains an invalid reason-separated belief", name)
			}
		}
	}
	for _, classification := range projection.Classifications {
		if classification.ActionID < 0 ||
			strings.TrimSpace(classification.Classifier) == "" {
			return fmt.Errorf("%s contains an incomplete action classification", name)
		}
		if consensus && !classification.Follow && !classification.Violation {
			return fmt.Errorf("%s contains a non-universal action classification", name)
		}
	}
	for _, value := range projection.SemanticValues {
		if value.ActionID < 0 ||
			strings.TrimSpace(value.Category) == "" ||
			strings.TrimSpace(value.Name) == "" {
			return fmt.Errorf("%s contains an incomplete semantic value", name)
		}
	}
	for _, connection := range projection.PlayConnections {
		if connection.SourceTransition < 0 ||
			connection.AvailableFromBoundary < 0 ||
			connection.ProvenanceID < 0 ||
			connection.FocusCardID < 0 ||
			connection.FocusIdentityMask <= 0 ||
			connection.Prerequisites == nil {
			return fmt.Errorf("%s contains an incomplete Play Connection", name)
		}
		if err := validateResearchHSMConnectionCards(
			connection.Prerequisites,
			name+" Play Connection prerequisites",
			false,
		); err != nil {
			return err
		}
	}
	for _, obligation := range projection.ConnectionObligations {
		if obligation.SourceTransition < 0 ||
			obligation.AvailableFromBoundary < 0 ||
			obligation.ProvenanceID < 0 ||
			(obligation.Kind != "prompt" && obligation.Kind != "finesse") ||
			obligation.OwnerPlayer < 0 ||
			obligation.FocusCardID < 0 ||
			obligation.FocusIdentityMask <= 0 {
			return fmt.Errorf("%s contains an incomplete Connection Obligation", name)
		}
		if err := validateResearchHSMConnectionCards(
			obligation.Candidates,
			name+" Connection Obligation candidates",
			true,
		); err != nil {
			return err
		}
		if obligation.CurrentCandidateIndex < 0 ||
			obligation.CurrentCandidateIndex >= len(obligation.Candidates) {
			return fmt.Errorf("%s Connection Obligation current candidate is invalid", name)
		}
	}
	return nil
}

func validateResearchHSMConnectionCards(
	cards []ResearchHSMConnectionCard,
	name string,
	requireNonempty bool,
) error {
	if cards == nil || (requireNonempty && len(cards) == 0) {
		return fmt.Errorf("%s must be explicitly complete", name)
	}
	for _, card := range cards {
		if card.StableCardID < 0 || card.IdentityMask <= 0 {
			return fmt.Errorf("%s contains an invalid card", name)
		}
	}
	return nil
}

type ResearchHSMPhysicalGuard struct {
	WorldIDs          []int    `json:"world_ids"`
	ClauseIDs         []string `json:"clause_ids"`
	EvidenceBoundary  int      `json:"evidence_boundary"`
	SemanticProfileID int      `json:"semantic_profile_id"`
}

type ResearchHSMDiagnosis struct {
	Label         string                   `json:"label"`
	PhysicalGuard ResearchHSMPhysicalGuard `json:"physical_guard"`
	ResearchHSMDiagnosticProjection
}

type ResearchHSMViolationWarning struct {
	ActionID       int `json:"action_id"`
	DiagnosisCount int `json:"diagnosis_count"`
	TotalDiagnoses int `json:"total_diagnoses"`
}

type ResearchHSMMistakenAction struct {
	TransitionIndex      int      `json:"transition_index"`
	HistoricalActor      int      `json:"historical_actor"`
	ActionID             int      `json:"action_id"`
	ViolatingClassifiers []string `json:"violating_classifiers"`
}

type ResearchHSMActionTimeClassification struct {
	GenerationID      uint32 `json:"generation_id"`
	TargetBoundary    int    `json:"target_boundary"`
	EvidenceBoundary  int    `json:"evidence_boundary"`
	PerspectivePlayer int    `json:"perspective_player"`
	SemanticProfileID int    `json:"semantic_profile_id"`
	SelectedActionID  int    `json:"selected_action_id"`
	FinalFollow       bool   `json:"final_follow"`
	FinalViolation    bool   `json:"final_violation"`
	RuleFollow        []bool `json:"rule_follow"`
	RuleViolation     []bool `json:"rule_violation"`
}

// ResearchHSMSnapshot contains semantics only. Transport identity and Physical
// Truth are deliberately carried by separate envelopes.
type ResearchHSMSnapshot struct {
	GenerationID                   uint32                               `json:"generation_id"`
	TargetBoundary                 int                                  `json:"target_boundary"`
	EvidenceBoundary               int                                  `json:"evidence_boundary"`
	PerspectivePlayer              int                                  `json:"perspective_player"`
	SemanticProgramID              string                               `json:"semantic_program_id"`
	SemanticProfileID              int                                  `json:"semantic_profile_id"`
	AuthorityLegalActionProjection []bool                               `json:"authority_legal_action_projection"`
	AggregateActionClassifications []ResearchHSMClassification          `json:"aggregate_action_classifications"`
	MistakenActions                []ResearchHSMMistakenAction          `json:"mistaken_actions"`
	Diagnoses                      []ResearchHSMDiagnosis               `json:"diagnoses"`
	Consensus                      ResearchHSMDiagnosticProjection      `json:"consensus"`
	ViolationWarnings              []ResearchHSMViolationWarning        `json:"violation_warnings"`
	ActionTimeClassification       *ResearchHSMActionTimeClassification `json:"action_time_classification"`
	PlainText                      string                               `json:"plain_text"`
	raw                            json.RawMessage
}

// UnmarshalJSON retains the Python-owned semantic DTO verbatim. Typed fields
// remain only for legacy in-process fixtures; production transport never
// reinterprets or reconstructs the inner payload.
func (snapshot *ResearchHSMSnapshot) UnmarshalJSON(data []byte) error {
	type snapshotAlias ResearchHSMSnapshot
	var decoded snapshotAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*snapshot = ResearchHSMSnapshot(decoded)
	snapshot.raw = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON forwards the exact Python payload when it came from transport.
func (snapshot ResearchHSMSnapshot) MarshalJSON() ([]byte, error) {
	if len(snapshot.raw) != 0 {
		return snapshot.raw, nil
	}
	type snapshotAlias ResearchHSMSnapshot
	return json.Marshal(snapshotAlias(snapshot))
}

func (snapshot ResearchHSMSnapshot) validateForRequest(
	request *ResearchHSMSnapshotRequest,
) error {
	if snapshot.GenerationID == 0 ||
		snapshot.GenerationID != request.ArchiveGenerationID ||
		snapshot.TargetBoundary != request.TargetBoundary ||
		snapshot.EvidenceBoundary != request.EvidenceBoundary ||
		snapshot.PerspectivePlayer != request.PerspectivePlayer ||
		snapshot.SemanticProfileID != request.SemanticProfileID {
		return fmt.Errorf("HSM snapshot coordinate does not match its pending request")
	}
	expectedLegality := request.AuthorityLegalProjection.legality()
	if len(snapshot.AuthorityLegalActionProjection) != len(expectedLegality) {
		return fmt.Errorf("HSM snapshot authority legal projection has the wrong fixed schema")
	}
	for actionID, legal := range expectedLegality {
		if snapshot.AuthorityLegalActionProjection[actionID] != legal {
			return fmt.Errorf("HSM snapshot authority legal projection does not match its pending request")
		}
	}
	if snapshot.AggregateActionClassifications == nil {
		return fmt.Errorf("HSM snapshot aggregate classifications must be explicit")
	}
	for _, classification := range snapshot.AggregateActionClassifications {
		if classification.ActionID < 0 ||
			strings.TrimSpace(classification.Classifier) == "" {
			return fmt.Errorf("HSM snapshot contains an incomplete aggregate classification")
		}
	}
	if snapshot.MistakenActions == nil {
		return fmt.Errorf("HSM snapshot Mistaken Actions must be explicit")
	}
	for _, mistaken := range snapshot.MistakenActions {
		if mistaken.TransitionIndex < 0 ||
			mistaken.HistoricalActor < 0 ||
			mistaken.ActionID < 0 ||
			len(mistaken.ViolatingClassifiers) == 0 {
			return fmt.Errorf("HSM snapshot contains an incomplete Mistaken Action")
		}
		for _, classifier := range mistaken.ViolatingClassifiers {
			if strings.TrimSpace(classifier) == "" {
				return fmt.Errorf("HSM snapshot Mistaken Action classifier is empty")
			}
		}
	}
	if len(snapshot.Diagnoses) == 0 {
		return fmt.Errorf("successful HSM snapshot requires at least one correlated diagnosis")
	}
	labels := make(map[string]bool, len(snapshot.Diagnoses))
	worldIDs := make(map[int]bool)
	for _, diagnosis := range snapshot.Diagnoses {
		label := strings.TrimSpace(diagnosis.Label)
		if !researchHSMDiagnosisLabel.MatchString(label) {
			return fmt.Errorf("every HSM diagnosis requires a canonical label")
		}
		if labels[label] {
			return fmt.Errorf("HSM diagnosis label %q is duplicated", label)
		}
		labels[label] = true
		if err := diagnosis.PhysicalGuard.validateForRequest(request); err != nil {
			return fmt.Errorf("HSM diagnosis %s: %w", label, err)
		}
		for _, worldID := range diagnosis.PhysicalGuard.WorldIDs {
			if worldIDs[worldID] {
				return fmt.Errorf("HSM physical guard World ID %d is shared by diagnoses", worldID)
			}
			worldIDs[worldID] = true
		}
		if err := diagnosis.ResearchHSMDiagnosticProjection.validate(
			"HSM diagnosis "+label,
			false,
		); err != nil {
			return err
		}
	}
	if err := snapshot.Consensus.validate("HSM consensus", true); err != nil {
		return err
	}
	if snapshot.ViolationWarnings == nil {
		return fmt.Errorf("HSM snapshot violation warnings must be explicit")
	}
	for _, warning := range snapshot.ViolationWarnings {
		if warning.ActionID < 0 ||
			warning.TotalDiagnoses != len(snapshot.Diagnoses) ||
			warning.DiagnosisCount <= 0 ||
			warning.DiagnosisCount > warning.TotalDiagnoses {
			return fmt.Errorf("HSM snapshot contains an invalid existential violation warning")
		}
	}
	actionTime := snapshot.ActionTimeClassification
	if actionTime != nil &&
		(actionTime.GenerationID != request.ArchiveGenerationID ||
			actionTime.TargetBoundary != request.TargetBoundary ||
			actionTime.EvidenceBoundary != actionTime.TargetBoundary ||
			actionTime.PerspectivePlayer != request.PerspectivePlayer ||
			actionTime.SemanticProfileID != snapshot.SemanticProfileID) {
		return fmt.Errorf("HSM action-time classification coordinate does not match response identity")
	}
	if actionTime != nil &&
		(actionTime.SelectedActionID < 0 ||
			actionTime.RuleFollow == nil ||
			actionTime.RuleViolation == nil ||
			len(actionTime.RuleFollow) != len(actionTime.RuleViolation)) {
		return fmt.Errorf("HSM action-time classification is incomplete")
	}
	if strings.TrimSpace(snapshot.PlainText) == "" {
		return fmt.Errorf("HSM snapshot plainText is required")
	}
	return nil
}

func (guard ResearchHSMPhysicalGuard) validateForRequest(
	request *ResearchHSMSnapshotRequest,
) error {
	if guard.WorldIDs == nil || len(guard.WorldIDs) == 0 {
		return fmt.Errorf("physical guard requires at least one world")
	}
	worldIDs := make(map[int]bool, len(guard.WorldIDs))
	for _, worldID := range guard.WorldIDs {
		if worldID < 0 {
			return fmt.Errorf("physical guard contains an invalid world")
		}
		if worldIDs[worldID] {
			return fmt.Errorf("physical guard duplicates World ID %d", worldID)
		}
		worldIDs[worldID] = true
	}
	if guard.EvidenceBoundary != request.EvidenceBoundary {
		return fmt.Errorf("physical guard evidence boundary does not match its pending request")
	}
	if guard.SemanticProfileID != request.SemanticProfileID {
		return fmt.Errorf("physical guard semantic profile does not match its pending request")
	}
	return nil
}

type ResearchHSMUnsatisfiableCore struct {
	Count            int    `json:"count"`
	Valid            []bool `json:"valid"`
	CoordinateKind   []int  `json:"coordinate_kind"`
	TransitionIndex  []int  `json:"transition_index"`
	RuleIndex        []int  `json:"rule_index"`
	SubjectIndex     []int  `json:"subject_index"`
	EvidenceBoundary []int  `json:"evidence_boundary"`
	ProvenanceID     []int  `json:"provenance_id"`
	SubsetMinimal    bool   `json:"subset_minimal"`
}

type ResearchHSMInvariantFailure struct {
	PrimaryDefect    int    `json:"primary_defect"`
	Count            int    `json:"count"`
	Valid            []bool `json:"valid"`
	Defect           []int  `json:"defect"`
	TransitionIndex  []int  `json:"transition_index"`
	RuleIndex        []int  `json:"rule_index"`
	SubjectIndex     []int  `json:"subject_index"`
	EvidenceBoundary []int  `json:"evidence_boundary"`
	ProvenanceID     []int  `json:"provenance_id"`
}

type ResearchHSMFailure struct {
	Category              string                        `json:"category"`
	Phase                 string                        `json:"phase"`
	TopologyID            int                           `json:"topology_id"`
	CapacityManifestID    string                        `json:"capacity_manifest_id"`
	SemanticProgramID     string                        `json:"semantic_program_id"`
	SemanticProfileID     int                           `json:"semantic_profile_id"`
	LegalActionProjection []bool                        `json:"legal_action_projection"`
	UnsatisfiableCore     *ResearchHSMUnsatisfiableCore `json:"unsatisfiable_core,omitempty"`
	InvariantFailure      *ResearchHSMInvariantFailure  `json:"invariant_failure,omitempty"`
	raw                   json.RawMessage
}

// UnmarshalJSON retains the Python-owned typed failure verbatim. The server
// authenticates and correlates the outer envelope; Python owns this semantic
// payload's schema and validation.
func (failure *ResearchHSMFailure) UnmarshalJSON(data []byte) error {
	type failureHeader struct {
		Category              string `json:"category"`
		Phase                 string `json:"phase"`
		TopologyID            int    `json:"topology_id"`
		CapacityManifestID    string `json:"capacity_manifest_id"`
		SemanticProgramID     string `json:"semantic_program_id"`
		SemanticProfileID     int    `json:"semantic_profile_id"`
		LegalActionProjection []bool `json:"legal_action_projection"`
	}
	var header failureHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	*failure = ResearchHSMFailure{
		Category:              header.Category,
		Phase:                 header.Phase,
		TopologyID:            header.TopologyID,
		CapacityManifestID:    header.CapacityManifestID,
		SemanticProgramID:     header.SemanticProgramID,
		SemanticProfileID:     header.SemanticProfileID,
		LegalActionProjection: header.LegalActionProjection,
		raw:                   append(json.RawMessage(nil), data...),
	}
	return nil
}

// MarshalJSON forwards the exact Python payload when it came from transport.
func (failure ResearchHSMFailure) MarshalJSON() ([]byte, error) {
	if len(failure.raw) != 0 {
		return failure.raw, nil
	}
	type failureAlias ResearchHSMFailure
	return json.Marshal(failureAlias(failure))
}

func (failure ResearchHSMFailure) validateForRequest(request *ResearchHSMSnapshotRequest) error {
	switch failure.Category {
	case "observer_evidence_unsatisfiable",
		"semantic_program_unsatisfiable",
		"structural_invariant_failure":
	default:
		return fmt.Errorf("HSM failure category is not canonical")
	}
	switch failure.Phase {
	case "archive_projection",
		"semantic_compilation",
		"exact_solving",
		"result_projection",
		"structural_validation":
	default:
		return fmt.Errorf("HSM failure phase is not canonical")
	}
	if failure.SemanticProfileID != request.SemanticProfileID {
		return fmt.Errorf("HSM failure semantic profile does not match its pending request")
	}
	if failure.TopologyID < 0 ||
		strings.TrimSpace(failure.CapacityManifestID) == "" ||
		!researchHSMSemanticProgramID.MatchString(failure.SemanticProgramID) {
		return fmt.Errorf("HSM failure generated provenance is incomplete")
	}
	expectedLegality := request.AuthorityLegalProjection.legality()
	if len(failure.LegalActionProjection) != len(expectedLegality) {
		return fmt.Errorf("HSM failure legal projection has the wrong fixed schema")
	}
	for actionID, legal := range expectedLegality {
		if failure.LegalActionProjection[actionID] != legal {
			return fmt.Errorf("HSM failure legal projection does not match its pending request")
		}
	}
	if failure.UnsatisfiableCore != nil && failure.InvariantFailure != nil {
		return fmt.Errorf("HSM failure cannot publish multiple mutually exclusive causes")
	}
	switch failure.Category {
	case "observer_evidence_unsatisfiable", "semantic_program_unsatisfiable":
		if failure.UnsatisfiableCore == nil || failure.InvariantFailure != nil {
			return fmt.Errorf("unsatisfiable HSM failure requires only an unsatisfiable core")
		}
		if err := failure.UnsatisfiableCore.validate(); err != nil {
			return err
		}
	case "structural_invariant_failure":
		if failure.InvariantFailure == nil || failure.UnsatisfiableCore != nil {
			return fmt.Errorf("invariant HSM failure requires only invariant provenance")
		}
		if err := failure.InvariantFailure.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (core ResearchHSMUnsatisfiableCore) validate() error {
	length := len(core.Valid)
	if core.Count < 0 || core.Count > length ||
		core.Valid == nil ||
		core.CoordinateKind == nil ||
		core.TransitionIndex == nil ||
		core.RuleIndex == nil ||
		core.SubjectIndex == nil ||
		core.EvidenceBoundary == nil ||
		core.ProvenanceID == nil ||
		len(core.CoordinateKind) != length ||
		len(core.TransitionIndex) != length ||
		len(core.RuleIndex) != length ||
		len(core.SubjectIndex) != length ||
		len(core.EvidenceBoundary) != length ||
		len(core.ProvenanceID) != length {
		return fmt.Errorf("HSM failure unsatisfiable core has an incomplete fixed schema")
	}
	return nil
}

func (failure ResearchHSMInvariantFailure) validate() error {
	length := len(failure.Valid)
	if failure.Count < 0 || failure.Count > length ||
		failure.Valid == nil ||
		failure.Defect == nil ||
		failure.TransitionIndex == nil ||
		failure.RuleIndex == nil ||
		failure.SubjectIndex == nil ||
		failure.EvidenceBoundary == nil ||
		failure.ProvenanceID == nil ||
		len(failure.Defect) != length ||
		len(failure.TransitionIndex) != length ||
		len(failure.RuleIndex) != length ||
		len(failure.SubjectIndex) != length ||
		len(failure.EvidenceBoundary) != length ||
		len(failure.ProvenanceID) != length {
		return fmt.Errorf("HSM failure invariant provenance has an incomplete fixed schema")
	}
	return nil
}

func responseIdentityForSnapshotRequest(request *ResearchHSMSnapshotRequest) ResearchHSMResponseIdentity {
	return ResearchHSMResponseIdentity{
		ProtocolVersion:                ResearchHSMProtocolVersion,
		ServerRequestID:                request.ServerRequestID,
		ClientRequestID:                request.ClientRequestID,
		ArchiveGenerationID:            request.ArchiveGenerationID,
		TargetBoundary:                 request.TargetBoundary,
		EvidenceBoundary:               request.EvidenceBoundary,
		PerspectivePlayer:              request.PerspectivePlayer,
		ActorPlayer:                    request.ActorPlayer,
		SemanticProfileID:              request.SemanticProfileID,
		AuthorityLegalProjectionDigest: request.AuthorityLegalProjection.Digest,
	}
}

func (identity ResearchHSMResponseIdentity) matchesSnapshotRequest(request *ResearchHSMSnapshotRequest) bool {
	return identity == responseIdentityForSnapshotRequest(request)
}

func hsmResponseIdentityMessage(identity ResearchHSMResponseIdentity) map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion":                identity.ProtocolVersion,
		"serverRequestID":                identity.ServerRequestID,
		"clientRequestID":                identity.ClientRequestID,
		"archiveGenerationID":            identity.ArchiveGenerationID,
		"targetBoundary":                 identity.TargetBoundary,
		"evidenceBoundary":               identity.EvidenceBoundary,
		"perspectivePlayer":              identity.PerspectivePlayer,
		"actorPlayer":                    identity.ActorPlayer,
		"semanticProfileID":              identity.SemanticProfileID,
		"authorityLegalProjectionDigest": identity.AuthorityLegalProjectionDigest,
	}
}

type ResearchHSMPhysicalTruthIdentity struct {
	ProtocolVersion     int    `json:"protocol_version"`
	ServerRequestID     int    `json:"server_request_id"`
	ClientRequestID     int    `json:"client_request_id"`
	ArchiveGenerationID uint32 `json:"archive_generation_id"`
	TargetBoundary      int    `json:"target_boundary"`
	PerspectivePlayer   int    `json:"perspective_player"`
}

type ResearchHSMPhysicalTruthCard struct {
	StableCardID int `json:"stableCardID"`
	Identity     int `json:"identity"`
}

// ResearchHSMPhysicalTruthOverlay is presentation-only authority truth. It is
// never embedded in an observer-relative semantic snapshot.
type ResearchHSMPhysicalTruthOverlay struct {
	Cards []ResearchHSMPhysicalTruthCard `json:"cards"`
}

func (overlay ResearchHSMPhysicalTruthOverlay) validate() error {
	if overlay.Cards == nil || len(overlay.Cards) == 0 {
		return fmt.Errorf("Physical Truth requires an explicit non-empty card overlay")
	}
	stableCardIDs := make(map[int]bool, len(overlay.Cards))
	for _, card := range overlay.Cards {
		if card.StableCardID < 0 || card.Identity < 0 || card.Identity >= 25 {
			return fmt.Errorf("Physical Truth contains an invalid card identity")
		}
		if stableCardIDs[card.StableCardID] {
			return fmt.Errorf("Physical Truth duplicates Stable Card ID %d", card.StableCardID)
		}
		stableCardIDs[card.StableCardID] = true
	}
	return nil
}

func physicalTruthIdentityForRequest(request *ResearchHSMPhysicalTruthRequest) ResearchHSMPhysicalTruthIdentity {
	return ResearchHSMPhysicalTruthIdentity{
		ProtocolVersion:     ResearchHSMProtocolVersion,
		ServerRequestID:     request.ServerRequestID,
		ClientRequestID:     request.ClientRequestID,
		ArchiveGenerationID: request.ArchiveGenerationID,
		TargetBoundary:      request.TargetBoundary,
		PerspectivePlayer:   request.PerspectivePlayer,
	}
}

func (identity ResearchHSMPhysicalTruthIdentity) matchesPhysicalTruthRequest(request *ResearchHSMPhysicalTruthRequest) bool {
	return identity == physicalTruthIdentityForRequest(request)
}

func hsmPhysicalTruthIdentityMessage(identity ResearchHSMPhysicalTruthIdentity) map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion":     identity.ProtocolVersion,
		"serverRequestID":     identity.ServerRequestID,
		"clientRequestID":     identity.ClientRequestID,
		"archiveGenerationID": identity.ArchiveGenerationID,
		"targetBoundary":      identity.TargetBoundary,
		"perspectivePlayer":   identity.PerspectivePlayer,
	}
}
