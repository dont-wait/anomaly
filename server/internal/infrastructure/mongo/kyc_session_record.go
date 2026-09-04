package mongo

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type MediaObjectRecord struct {
	StorageKey *string `bson:"storage_key"`
	MIMEType   *string `bson:"mime_type"`
	SizeBytes  *int64  `bson:"size_bytes"`
	SHA256     *string `bson:"sha256"`
}

type livenessVideoRecord struct {
	MediaObjectRecord `bson:",inline"`
	DurationSeconds   *int `bson:"duration_seconds"`
}

type kycIdentityDataRecord struct {
	Type             *string    `bson:"type"`
	Number           *string    `bson:"number"`
	FullName         *string    `bson:"full_name"`
	DateOfBirth      *time.Time `bson:"date_of_birth"`
	IssuedDate       *time.Time `bson:"issued_date"`
	IssuedPlace      *string    `bson:"issued_place"`
	PermanentAddress *string    `bson:"permanent_address"`
}

type kycMediaRecord struct {
	IdentityFront MediaObjectRecord   `bson:"identity_front"`
	IdentityBack  MediaObjectRecord   `bson:"identity_back"`
	FaceImage     MediaObjectRecord   `bson:"face_image"`
	LivenessVideo livenessVideoRecord `bson:"liveness_video"`
}

type kycVerificationRecord struct {
	OCRStatus           string           `bson:"ocr_status"`
	LivenessStatus      string           `bson:"liveness_status"`
	FaceMatchStatus     string           `bson:"face_match_status"`
	FaceMatchScore      *bson.Decimal128 `bson:"face_match_score"`
	Provider            *string          `bson:"provider"`
	ProviderReferenceId *string          `bson:"provider_reference_id"`
}

type kycFailureRecord struct {
	Code    *string `bson:"code"`
	Message *string `bson:"message"`
}

type kycSessionRecord struct {
	Id           bson.ObjectID         `bson:"_id"`
	CustomerId   bson.ObjectID         `bson:"customer_id"`
	AttemptNo    int                   `bson:"attempt_no"`
	Status       string                `bson:"status"`
	IdentityData kycIdentityDataRecord `bson:"identity_data"`
	Media        kycMediaRecord        `bson:"media"`
	Verification kycVerificationRecord `bson:"verification"`
	Failure      kycFailureRecord      `bson:"failure"`
	StartedAt    time.Time             `bson:"started_at"`
	CompletedAt  *time.Time            `bson:"completed_at"`
	CreatedAt    time.Time             `bson:"created_at"`
}

func toKYCSessionRecord(session *accountdomain.KYCSession) (kycSessionRecord, error) {
	id, err := bson.ObjectIDFromHex(session.Id)
	if err != nil {
		return kycSessionRecord{}, fmt.Errorf("invalid KYC session id %q: %w", session.Id, err)
	}
	customerID, err := bson.ObjectIDFromHex(session.CustomerId)
	if err != nil {
		return kycSessionRecord{}, fmt.Errorf("invalid customer id %q: %w", session.CustomerId, err)
	}

	var score *bson.Decimal128
	if session.Verification.FaceMatchScore != "" {
		value, err := bson.ParseDecimal128(session.Verification.FaceMatchScore)
		if err != nil {
			return kycSessionRecord{}, fmt.Errorf("invalid face match score %q: %w", session.Verification.FaceMatchScore, err)
		}
		score = &value
	}

	toMedia := func(media accountdomain.MediaObject) MediaObjectRecord {
		return MediaObjectRecord{
			StorageKey: optionalString(media.StorageKey),
			MIMEType:   optionalString(media.MIMEType),
			SizeBytes:  optionalInt64(media.SizeBytes),
			SHA256:     optionalString(media.SHA256),
		}
	}

	failure := kycFailureRecord{}
	if session.Failure != nil {
		failure = kycFailureRecord{
			Code:    optionalString(session.Failure.Code),
			Message: optionalString(session.Failure.Message),
		}
	}

	return kycSessionRecord{
		Id:         id,
		CustomerId: customerID,
		AttemptNo:  session.AttemptNo,
		Status:     string(session.Status),
		IdentityData: kycIdentityDataRecord{
			Type:             optionalString(session.IdentityData.Type),
			Number:           optionalString(session.IdentityData.Number),
			FullName:         optionalString(session.IdentityData.FullName),
			DateOfBirth:      session.IdentityData.DateOfBirth,
			IssuedDate:       session.IdentityData.IssuedDate,
			IssuedPlace:      optionalString(session.IdentityData.IssuedPlace),
			PermanentAddress: optionalString(session.IdentityData.PermanentAddress),
		},
		Media: kycMediaRecord{
			IdentityFront: toMedia(session.Media.IdentityFront),
			IdentityBack:  toMedia(session.Media.IdentityBack),
			FaceImage:     toMedia(session.Media.FaceImage),
			LivenessVideo: livenessVideoRecord{
				MediaObjectRecord: toMedia(session.Media.LivenessVideo.MediaObject),
				DurationSeconds:   optionalInt(session.Media.LivenessVideo.DurationSeconds),
			},
		},
		Verification: kycVerificationRecord{
			OCRStatus:           string(session.Verification.OCRStatus),
			LivenessStatus:      string(session.Verification.LivenessStatus),
			FaceMatchStatus:     string(session.Verification.FaceMatchStatus),
			FaceMatchScore:      score,
			Provider:            optionalString(session.Verification.Provider),
			ProviderReferenceId: optionalString(session.Verification.ProviderReferenceId),
		},
		Failure:     failure,
		StartedAt:   session.StartedAt,
		CompletedAt: session.CompletedAt,
		CreatedAt:   session.CreatedAt,
	}, nil
}

func optionalInt64(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}
