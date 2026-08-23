package authority

// Presence preserves absent, empty, and present values without using a zero
// value as security meaning.
type Presence uint8

const (
	PresenceInvalid Presence = iota
	PresenceAbsent
	PresenceEmpty
	PresencePresent
)

type RevocationStatus uint8

const (
	RevocationInvalid RevocationStatus = iota
	RevocationUnknown
	RevocationNotRevoked
	RevocationRevoked
)

type EligibilityResult uint8

const (
	EligibilityInvalid EligibilityResult = iota
	EligibilityUnknown
	EligibilityIneligible
	EligibilityEligible
)

type ReviewState uint8

const (
	ReviewInvalid ReviewState = iota
	ReviewDraft
	ReviewReviewed
	ReviewApproved
	ReviewRejected
)

type AuthorizationState uint8

const (
	AuthorizationInvalid AuthorizationState = iota
	AuthorizationUnknown
	AuthorizationNotAuthorized
	AuthorizationAuthorized
)

type MaterializationState uint8

const (
	MaterializationInvalid MaterializationState = iota
	MaterializationUnknown
	MaterializationNotMaterialized
	MaterializationMaterialized
)

type RuntimeAuthorityState uint8

const (
	RuntimeInvalid RuntimeAuthorityState = iota
	RuntimeNotActive
	RuntimeActive
	RuntimeActiveUnauthorized
	RuntimeSuspendedUnknown
)

type BehavioralVerificationState uint8

const (
	BehavioralInvalid BehavioralVerificationState = iota
	BehavioralNotVerified
	BehavioralVerified
	BehavioralUnknown
)

func (s EligibilityResult) Valid() bool { return s >= EligibilityUnknown && s <= EligibilityEligible }
func (s RevocationStatus) Valid() bool  { return s >= RevocationUnknown && s <= RevocationRevoked }
func (s ReviewState) Valid() bool       { return s >= ReviewDraft && s <= ReviewRejected }
func (s AuthorizationState) Valid() bool {
	return s >= AuthorizationUnknown && s <= AuthorizationAuthorized
}
func (s MaterializationState) Valid() bool {
	return s >= MaterializationUnknown && s <= MaterializationMaterialized
}
func (s RuntimeAuthorityState) Valid() bool {
	return s >= RuntimeNotActive && s <= RuntimeSuspendedUnknown
}
func (s BehavioralVerificationState) Valid() bool {
	return s >= BehavioralNotVerified && s <= BehavioralUnknown
}
