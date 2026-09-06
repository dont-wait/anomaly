package mongo

import (
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type MediaObjectRecord struct {
	StorageKey string  `bson:"storage_key"`
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
	FaceImage     *MediaObjectRecord  `bson:"face_image,omitempty"`
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

	toMedia := func(field string, media accountdomain.MediaObject) (MediaObjectRecord, error) {
		if strings.TrimSpace(media.StorageKey) == "" {
			return MediaObjectRecord{}, fmt.Errorf("%s storage key is required", field)
		}
		return MediaObjectRecord{
			StorageKey: media.StorageKey,
			MIMEType:   optionalString(media.MIMEType),
			SizeBytes:  optionalInt64(media.SizeBytes),
			SHA256:     optionalString(media.SHA256),
		}, nil
	}
	identityFront, err := toMedia("identity front", session.Media.IdentityFront)
	if err != nil {
		return kycSessionRecord{}, err
	}
	identityBack, err := toMedia("identity back", session.Media.IdentityBack)
	if err != nil {
		return kycSessionRecord{}, err
	}
	livenessVideo, err := toMedia("liveness video", session.Media.LivenessVideo.MediaObject)
	if err != nil {
		return kycSessionRecord{}, err
	}
	var faceImage *MediaObjectRecord
	if session.Media.FaceImage != nil {
		record, err := toMedia("face image", *session.Media.FaceImage)
		if err != nil {
			return kycSessionRecord{}, err
		}
		faceImage = &record
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
			IdentityFront: identityFront,
			IdentityBack:  identityBack,
			FaceImage:     faceImage,
			LivenessVideo: livenessVideoRecord{
				MediaObjectRecord: livenessVideo,
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

func fromKYCSessionRecord(record kycSessionRecord) *accountdomain.KYCSession {
	toMedia := func(media MediaObjectRecord) accountdomain.MediaObject {
		return accountdomain.MediaObject{
			StorageKey: media.StorageKey,
			MIMEType:   stringValue(media.MIMEType),
			SizeBytes:  int64Value(media.SizeBytes),
			SHA256:     stringValue(media.SHA256),
		}
	}
	var faceImage *accountdomain.MediaObject
	if record.Media.FaceImage != nil {
		media := toMedia(*record.Media.FaceImage)
		faceImage = &media
	}

	var failure *accountdomain.KYCFailure
	if record.Failure.Code != nil || record.Failure.Message != nil {
		failure = &accountdomain.KYCFailure{
			Code:    stringValue(record.Failure.Code),
			Message: stringValue(record.Failure.Message),
		}
	}

	return &accountdomain.KYCSession{
		Id:         record.Id.Hex(),
		CustomerId: record.CustomerId.Hex(),
		AttemptNo:  record.AttemptNo,
		Status:     accountdomain.KYCSessionStatus(record.Status),
		IdentityData: accountdomain.KYCIdentityData{
			Type:             stringValue(record.IdentityData.Type),
			Number:           stringValue(record.IdentityData.Number),
			FullName:         stringValue(record.IdentityData.FullName),
			DateOfBirth:      record.IdentityData.DateOfBirth,
			IssuedDate:       record.IdentityData.IssuedDate,
			IssuedPlace:      stringValue(record.IdentityData.IssuedPlace),
			PermanentAddress: stringValue(record.IdentityData.PermanentAddress),
		},
		Media: accountdomain.KYCMedia{
			IdentityFront: toMedia(record.Media.IdentityFront),
			IdentityBack:  toMedia(record.Media.IdentityBack),
			FaceImage:     faceImage,
			LivenessVideo: accountdomain.LivenessVideo{
				MediaObject:     toMedia(record.Media.LivenessVideo.MediaObjectRecord),
				DurationSeconds: intValue(record.Media.LivenessVideo.DurationSeconds),
			},
		},
		Verification: accountdomain.KYCVerification{
			OCRStatus:           accountdomain.VerificationStatus(record.Verification.OCRStatus),
			LivenessStatus:      accountdomain.VerificationStatus(record.Verification.LivenessStatus),
			FaceMatchStatus:     accountdomain.VerificationStatus(record.Verification.FaceMatchStatus),
			FaceMatchScore:      decimalValue(record.Verification.FaceMatchScore),
			Provider:            stringValue(record.Verification.Provider),
			ProviderReferenceId: stringValue(record.Verification.ProviderReferenceId),
		},
		Failure:     failure,
		StartedAt:   record.StartedAt,
		CompletedAt: record.CompletedAt,
		CreatedAt:   record.CreatedAt,
	}
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func decimalValue(value *bson.Decimal128) string {
	if value == nil {
		return ""
	}
	return value.String()
}
