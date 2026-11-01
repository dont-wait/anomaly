package account

import "time"

type (
	AccountType        string
	AccountRole        string
	Currency           string
	AccountStatus      string
	CustomerStatus     string
	KYCStatus          string
	KYCSessionStatus   string
	VerificationStatus string
)

const (
	AccountTypePayment AccountType = "payment"
	AccountRoleUser    AccountRole = "user"
	AccountRoleAdmin   AccountRole = "admin"
	CurrencyVND        Currency    = "VND"

	AccountStatusActive  AccountStatus  = "active"
	CustomerStatusActive CustomerStatus = "active"

	KYCStatusNotStarted KYCStatus = "not_started"
	KYCStatusVerified   KYCStatus = "verified"

	KYCSessionStatusVerified KYCSessionStatus   = "verified"
	VerificationStatusNotRun VerificationStatus = "not_run"
	VerificationStatusPassed VerificationStatus = "passed"
	VerificationStatusFailed VerificationStatus = "failed"
)

type Balance struct {
	Current int64
}

type Address struct {
	Line         string
	ProvinceCode string
}

type CustomerProfile struct {
	FullName    string
	DateOfBirth *time.Time
	Phone       string
	Email       string
	Address     Address
}

type ProfileMedia struct {
	ID          string
	Type        string
	FileName    string
	ContentType string
	StorageKey  string
	SizeBytes   int64
	UploadedAt  time.Time
	Status      string
}

type CustomerIdentity struct {
	Type             string
	Number           string
	IssuedDate       *time.Time
	IssuedPlace      string
	PermanentAddress string
}

type CreditProfile struct {
	Score     int
	BadDebt   bool
	UpdatedAt time.Time
}

type Customer struct {
	Id                   string
	CustomerCode         string
	Profile              CustomerProfile
	Avatar               *ProfileMedia
	Documents            []ProfileMedia
	Identity             CustomerIdentity
	VerifiedKYCSessionId string
	KYCStatus            KYCStatus
	CreditProfile        CreditProfile
	Status               CustomerStatus
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type MediaObject struct {
	StorageKey string
	MIMEType   string
	SizeBytes  int64
	SHA256     string
}

type LivenessVideo struct {
	MediaObject
	DurationSeconds int
}

type KYCIdentityData struct {
	Type             string
	Number           string
	FullName         string
	DateOfBirth      *time.Time
	IssuedDate       *time.Time
	IssuedPlace      string
	PermanentAddress string
}

type KYCMedia struct {
	IdentityFront MediaObject
	IdentityBack  MediaObject
	FaceImage     *MediaObject
	LivenessVideo LivenessVideo
}

type KYCVerification struct {
	OCRStatus           VerificationStatus
	LivenessStatus      VerificationStatus
	FaceMatchStatus     VerificationStatus
	FaceMatchScore      string
	Provider            string
	ProviderReferenceId string
	LivenessScore       float64
	Evidence            *KYCEvidence
}

// KYCEvidence describes model execution, or an explicit poc-bypass record with not_run statuses.
type KYCEvidence struct {
	RequestID                string  `json:"request_id" bson:"request_id"`
	ModelID                  string  `json:"model_id" bson:"model_id"`
	ModelVersion             string  `json:"model_version" bson:"model_version"`
	ModelSHA256              string  `json:"model_sha256" bson:"model_sha256"`
	PipelineVersion          string  `json:"pipeline_version" bson:"pipeline_version"`
	CalibrationVersion       string  `json:"calibration_version" bson:"calibration_version"`
	MatchingStatus           string  `json:"matching_status" bson:"matching_status"`
	LivenessStatus           string  `json:"liveness_status" bson:"liveness_status"`
	SubjectConsistencyPassed bool    `json:"subject_consistency_passed" bson:"subject_consistency_passed"`
	ChallengePassed          bool    `json:"challenge_passed" bson:"challenge_passed"`
	SampledFrames            int     `json:"sampled_frames" bson:"sampled_frames"`
	MatchedFrames            int     `json:"matched_frames" bson:"matched_frames"`
	VideoDurationSeconds     float64 `json:"video_duration_seconds" bson:"video_duration_seconds"`
	ProcessingMS             int     `json:"processing_ms" bson:"processing_ms"`
}

type KYCFailure struct {
	Code    string
	Message string
}

type KYCSession struct {
	Id           string
	CustomerId   string
	AttemptNo    int
	Status       KYCSessionStatus
	IdentityData KYCIdentityData
	Media        KYCMedia
	Verification KYCVerification
	Failure      *KYCFailure
	StartedAt    time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
}

type UserAccount struct {
	Id           string
	AccountNo    string
	CustomerId   string
	Username     string
	Email        string
	PasswordHash string
	TokenVersion int64
	Type         AccountType
	Role         AccountRole
	Currency     Currency
	Balance      Balance
	Status       AccountStatus
	Version      int64
	OpenedAt     time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Customer     *Customer
	KYCSessions  []*KYCSession
}

func (u *UserAccount) EffectiveRole() AccountRole {
	if u.Role == "" {
		return AccountRoleUser
	}
	return u.Role
}

func (u *UserAccount) Withdraw(amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if amount > u.Balance.Current {
		return ErrInsufficientFunds
	}

	u.Balance.Current -= amount
	u.Version++
	u.UpdatedAt = time.Now().UTC()
	return nil
}

func (u *UserAccount) IsVerified() bool {
	return u.Customer != nil && u.Customer.KYCStatus == KYCStatusVerified
}

func (u *UserAccount) VerifiedKYCSession() *KYCSession {
	if u.Customer == nil || u.Customer.VerifiedKYCSessionId == "" {
		return nil
	}
	for _, session := range u.KYCSessions {
		if session.Id == u.Customer.VerifiedKYCSessionId {
			return session
		}
	}
	return nil
}
