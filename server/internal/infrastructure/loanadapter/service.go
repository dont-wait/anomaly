package loanadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	account "github.com/dont-wait/anomaly/internal/domain/account"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

type BankAccounts interface {
	FindByID(
		context.Context,
		string,
	) (*account.UserAccount, error)
}

type Access struct {
	Accounts BankAccounts
}

// CustomerID là UserID lấy từ JWT,
// không được lấy tùy ý từ request body.
func (a *Access) ValidateDisbursementAccount(
	ctx context.Context,
	userID string,
	targetID string,
) error {
	if a == nil ||
		a.Accounts == nil ||
		userID == "" ||
		targetID != userID {
		return fmt.Errorf(
			"loan destination must be the applicant's own payment account",
		)
	}

	owner, err := a.Accounts.FindByID(
		ctx,
		userID,
	)
	if err != nil {
		return err
	}

	if owner == nil ||
		owner.Status != account.AccountStatusActive ||
		owner.Type != account.AccountTypePayment ||
		owner.Currency != account.CurrencyVND ||
		!owner.IsVerified() ||
		owner.EffectiveRole() != account.AccountRoleUser {
		return fmt.Errorf(
			"active, verified personal payment account required",
		)
	}

	// Thông tin tài khoản nhận lương sẽ được
	// đối chiếu riêng với dữ liệu thẩm định.
	return nil
}

func (a *Access) requireAdmin(
	ctx context.Context,
	actorID string,
) error {
	if a == nil ||
		a.Accounts == nil ||
		actorID == "" {
		return fmt.Errorf(
			"loan admin authorization unavailable",
		)
	}

	actor, err := a.Accounts.FindByID(
		ctx,
		actorID,
	)
	if err != nil {
		return err
	}

	if actor == nil ||
		actor.Status != account.AccountStatusActive ||
		actor.EffectiveRole() != account.AccountRoleAdmin {
		return fmt.Errorf(
			"loan reviewer requires active admin role",
		)
	}

	return nil
}

func (a *Access) AuthorizeLoanReview(
	ctx context.Context,
	actorID string,
) error {
	return a.requireAdmin(ctx, actorID)
}

func (a *Access) AuthorizeLoanAssessment(
	ctx context.Context,
	actorID string,
) error {
	return a.requireAdmin(ctx, actorID)
}

func (a *Access) AuthorizeLoanManualDecision(
	ctx context.Context,
	actorID string,
) error {
	return a.requireAdmin(ctx, actorID)
}

// EvidenceStore lưu kết quả xác minh
// được nhân viên xác nhận trong MongoDB.
type EvidenceStore struct {
	col *mongodrv.Collection
}

func NewEvidenceStore(
	client *mongodrv.Client,
	dbName string,
) *EvidenceStore {
	return &EvidenceStore{
		col: client.Database(dbName).
			Collection("loan_verified_evidence"),
	}
}

func (s *EvidenceStore) Save(
	ctx context.Context,
	e loandomain.VerifiedEvidence,
) error {
	_, err := s.col.ReplaceOne(
		ctx,
		bson.M{
			"_id": e.ApplicationID,
		},
		bson.M{
			"_id":      e.ApplicationID,
			"evidence": e,
		},
		options.Replace().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("save loan evidence: %w", err)
	}

	return nil
}

func (s *EvidenceStore) Find(
	ctx context.Context,
	applicationID string,
) (loandomain.VerifiedEvidence, error) {
	var rec struct {
		Evidence loandomain.VerifiedEvidence `bson:"evidence"`
	}

	err := s.col.FindOne(
		ctx,
		bson.M{"_id": applicationID},
	).Decode(&rec)
	if err != nil {
		return loandomain.VerifiedEvidence{}, err
	}

	return rec.Evidence, nil
}

type AssessmentProvider struct {
	Evidence *EvidenceStore
}

func (p *AssessmentProvider) BuildAssessmentInput(
	ctx context.Context,
	app *loandomain.Application,
) (loandomain.LoanAssessmentInput, error) {
	if p == nil || p.Evidence == nil {
		return loandomain.LoanAssessmentInput{},
			fmt.Errorf("loan evidence service unavailable")
	}

	e, err := p.Evidence.Find(
		ctx,
		app.Id,
	)
	if err != nil {
		if errors.Is(err, mongodrv.ErrNoDocuments) {
			return loandomain.LoanAssessmentInput{},
				fmt.Errorf(
					"loan has no independently attested evidence",
				)
		}

		return loandomain.LoanAssessmentInput{}, err
	}

	input, _, err := e.BuildAssessment(
		app,
		time.Now().UTC(),
	)

	return input, err
}

// DocumentVerifier xác minh metadata
// với nội dung file thực tế trên RustFS.
type DocumentVerifier struct {
	Media *rustfs.MediaRepository
}

func (v *DocumentVerifier) VerifyLoanDocument(
	ctx context.Context,
	customerID string,
	applicationID string,
	doc loandomain.ApplicationDocument,
) error {
	if v == nil || v.Media == nil {
		return fmt.Errorf("loan media storage unavailable")
	}

	prefix := "loans/" +
		customerID + "/" +
		applicationID + "/"

	objectName := strings.TrimPrefix(
		doc.StorageKey,
		prefix,
	)

	if !strings.HasPrefix(doc.StorageKey, prefix) ||
		len(objectName) < 12 ||
		strings.Contains(objectName, "/") {
		return fmt.Errorf(
			"document storage key is outside owned application",
		)
	}

	if doc.SizeBytes <= 0 ||
		doc.SizeBytes > 10<<20 {
		return fmt.Errorf(
			"document size exceeds loan limit",
		)
	}

	result, err := v.Media.Download(
		ctx,
		doc.StorageKey,
	)
	if err != nil {
		return err
	}
	defer func() { _ = result.Body.Close() }()

	if result.ContentType != doc.MIMEType {
		return fmt.Errorf(
			"document media MIME mismatch",
		)
	}

	digest := sha256.New()

	n, err := io.Copy(
		digest,
		io.LimitReader(result.Body, (10<<20)+1),
	)
	if err != nil {
		return err
	}

	if n != doc.SizeBytes ||
		!strings.EqualFold(
			hex.EncodeToString(digest.Sum(nil)),
			doc.SHA256,
		) {
		return fmt.Errorf(
			"document content does not match registered metadata",
		)
	}

	return nil
}

// LoanContractEmail obtains a verified customer's contact email from canonical Banking.
func (a *Access) LoanContractEmail(ctx context.Context, customerID string) (string, error) {
	if a == nil || a.Accounts == nil || strings.TrimSpace(customerID) == "" {
		return "", fmt.Errorf("loan email verification unavailable")
	}
	u, err := a.Accounts.FindByID(ctx, customerID)
	if err != nil {
		return "", err
	}
	if u == nil || u.Id != customerID || u.Status != account.AccountStatusActive || !u.IsVerified() || u.EffectiveRole() != account.AccountRoleUser || strings.TrimSpace(u.Email) == "" {
		return "", fmt.Errorf("active verified customer email required")
	}
	return u.Email, nil
}

// VerifiedProfileDocuments trả CCCD đã xác minh qua KYC và các chứng từ đang
// hiệu lực trong hồ sơ cá nhân có thể dùng lại cho hồ sơ vay.
func (a *Access) VerifiedProfileDocuments(
	ctx context.Context,
	customerID string,
) ([]loandomain.ApplicationDocument, error) {
	if a == nil || a.Accounts == nil {
		return nil, fmt.Errorf("loan profile documents unavailable")
	}
	user, err := a.Accounts.FindByID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	var documents []loandomain.ApplicationDocument
	if session := user.VerifiedKYCSession(); session != nil {
		for _, item := range []struct {
			documentType string
			media        account.MediaObject
		}{
			{"IDENTITY_FRONT", session.Media.IdentityFront},
			{"IDENTITY_BACK", session.Media.IdentityBack},
		} {
			documents = append(documents, loandomain.ApplicationDocument{
				Type:       item.documentType,
				StorageKey: item.media.StorageKey,
				MIMEType:   item.media.MIMEType,
				SizeBytes:  item.media.SizeBytes,
				SHA256:     item.media.SHA256,
			})
		}
	}
	if user.Customer != nil {
		for _, media := range user.Customer.Documents {
			if media.Status != "ACTIVE" || !reusableProfileDocument(media.Type) {
				continue
			}
			documents = append(documents, loandomain.ApplicationDocument{
				Type:       media.Type,
				StorageKey: media.StorageKey,
				MIMEType:   media.ContentType,
				SizeBytes:  media.SizeBytes,
				SHA256:     media.SHA256,
			})
		}
	}
	return documents, nil
}

// CopyLoanDocument tạo bản sao riêng cho hồ sơ vay.
func (v *DocumentVerifier) CopyLoanDocument(
	ctx context.Context,
	sourceKey, targetKey, mimeType string,
) error {
	if v == nil || v.Media == nil {
		return fmt.Errorf("loan media storage unavailable")
	}
	source, err := v.Media.Download(ctx, sourceKey)
	if err != nil {
		return err
	}
	defer func() { _ = source.Body.Close() }()
	// S3 client cần body tua lại được, nên đọc vào bộ nhớ trong giới hạn chứng từ.
	data, err := io.ReadAll(io.LimitReader(source.Body, (10<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 10<<20 {
		return fmt.Errorf("document size exceeds loan limit")
	}
	return v.Media.Upload(ctx, targetKey, bytes.NewReader(data), mimeType)
}

func reusableProfileDocument(documentType string) bool {
	switch documentType {
	case "VNEID_PERSONAL", "VNEID_RESIDENCE", "VSSID_PARTICIPATION",
		"VSSID_DETAIL", "SALARY_STATEMENT_3_MONTHS":
		return true
	}
	return false
}
