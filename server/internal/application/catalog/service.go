// Package catalog điều phối vòng đời gói: nháp, phát hành, ngừng bán.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/dont-wait/anomaly/internal/domain/catalog"
	"github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/domain/savings"
)

var (
	ErrPackageNotFound = errors.New("package not found")
	ErrDraftExists     = errors.New("package already has a draft")
	ErrNoDraft         = errors.New("package has no draft")
	ErrVersionConflict = errors.New("package version changed; reload and retry")
	ErrInvalidRequest  = errors.New("invalid catalog request")
	ErrTermsNotFound   = errors.New("savings terms draft not found")
	ErrCodeExists      = errors.New("package code already exists")
	ErrPackageInUse    = errors.New("package has applications or contracts")
	ErrPackageActive   = errors.New("package is on sale; retire it first")
)

// AuditEntry là một dòng lịch sử thay đổi.
type AuditEntry struct {
	Action  string    `json:"action"`
	Version string    `json:"version"`
	ActorID string    `json:"actorId"`
	At      time.Time `json:"at"`
	Reason  string    `json:"reason,omitempty"`
}

// Transition mô tả một lần đổi trạng thái có idempotency.
type Transition struct {
	Kind            domain.Kind
	Code            string
	ExpectedVersion int
	ActorID         string
	Reason          string
	IdempotencyKey  string
	At              time.Time
}

// TermsDocument là một phiên bản điều khoản tiết kiệm.
type TermsDocument struct {
	Version      string                 `json:"version"`
	Status       string                 `json:"status"`
	Title        string                 `json:"title"`
	Introduction string                 `json:"introduction"`
	Sections     []savings.TermsSection `json:"sections"`
}

type Store interface {
	Versions(ctx context.Context, kind domain.Kind, code string) ([]domain.Package, error)
	ListAll(ctx context.Context, kind domain.Kind) ([]domain.Package, error)
	InsertDraft(ctx context.Context, draft domain.Package) error
	ReplaceDraft(ctx context.Context, draft domain.Package) error
	DeleteDraft(ctx context.Context, kind domain.Kind, code string) error
	// DeletePackage ẩn gói khỏi danh mục. Bản ghi được giữ lại để mã không bị dùng
	// lại cho sản phẩm khác và audit vẫn tra được.
	DeletePackage(ctx context.Context, kind domain.Kind, code, actorID string, at time.Time) error
	// Publish và Retire ghi thay đổi cùng audit trong một transaction.
	Publish(ctx context.Context, t Transition) (domain.Package, error)
	Retire(ctx context.Context, t Transition) (domain.Package, error)
	Usage(ctx context.Context, kind domain.Kind, code string) (map[string]int64, error)
	Audit(ctx context.Context, kind domain.Kind, code string, limit int) ([]AuditEntry, error)

	ListTerms(ctx context.Context) ([]TermsDocument, error)
	SaveTermsDraft(ctx context.Context, draft TermsDocument, actorID string, at time.Time) (TermsDocument, error)
	DeleteTermsDraft(ctx context.Context) error
	PublishTerms(ctx context.Context, version, actorID string, at time.Time) (TermsDocument, error)
}

type Service struct {
	Store Store
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Summary là một dòng trong danh sách gói.
type Summary struct {
	Kind          domain.Kind         `json:"kind"`
	Code          string              `json:"code"`
	Name          string              `json:"name"`
	Status        domain.Status       `json:"status"`
	ActiveVersion int                 `json:"activeVersion,omitempty"`
	DraftVersion  int                 `json:"draftVersion,omitempty"`
	MinAmount     int64               `json:"minAmount"`
	MaxAmount     int64               `json:"maxAmount,omitempty"`
	Terms         []domain.TermOption `json:"terms"`
}

type Detail struct {
	Summary
	Versions []domain.Package `json:"versions"`
	Usage    map[string]int64 `json:"usage"`
	Audit    []AuditEntry     `json:"audit"`
}

func summarize(kind domain.Kind, code string, versions []domain.Package) Summary {
	out := Summary{Kind: kind, Code: code, Status: domain.StatusSuperseded, Terms: []domain.TermOption{}}
	shown := false
	show := func(p domain.Package) {
		out.Name, out.MinAmount, out.MaxAmount, out.Terms = p.Name, p.MinAmount, p.MaxAmount, p.Terms
		shown = true
	}
	// Versions sắp theo phiên bản giảm dần.
	for _, p := range versions {
		switch p.Status {
		case domain.StatusActive:
			out.ActiveVersion, out.Status = p.Version, domain.StatusActive
			show(p)
		case domain.StatusDraft:
			out.DraftVersion = p.Version
		}
	}
	if out.ActiveVersion != 0 {
		return out
	}
	for _, p := range versions {
		if p.Status == domain.StatusRetired || p.Status == domain.StatusSuperseded {
			out.Status = domain.StatusRetired
			show(p)
			return out
		}
	}
	if !shown && len(versions) > 0 {
		out.Status = domain.StatusDraft
		show(versions[0])
	}
	return out
}

func (s *Service) List(ctx context.Context, kind domain.Kind, status, q string) ([]Summary, error) {
	all, err := s.Store.ListAll(ctx, kind)
	if err != nil {
		return nil, err
	}
	var order []string
	byCode := map[string][]domain.Package{}
	for _, p := range all {
		if _, ok := byCode[p.Code]; !ok {
			order = append(order, p.Code)
		}
		byCode[p.Code] = append(byCode[p.Code], p)
	}
	q = strings.ToLower(strings.TrimSpace(q))
	items := []Summary{}
	for _, code := range order {
		summary := summarize(kind, code, byCode[code])
		if status != "" && !strings.EqualFold(status, string(summary.Status)) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(summary.Code+" "+summary.Name), q) {
			continue
		}
		items = append(items, summary)
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, kind domain.Kind, code string) (Detail, error) {
	versions, err := s.Store.Versions(ctx, kind, code)
	if err != nil {
		return Detail{}, err
	}
	if len(versions) == 0 {
		return Detail{}, ErrPackageNotFound
	}
	usage, err := s.Store.Usage(ctx, kind, code)
	if err != nil {
		return Detail{}, err
	}
	audit, err := s.Store.Audit(ctx, kind, code, 50)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Summary: summarize(kind, code, versions), Versions: versions, Usage: usage, Audit: audit}, nil
}

// CreateInput là thông tin một gói mới; gói bắt đầu là bản nháp phiên bản 1.
type CreateInput struct {
	Code string `json:"code"`
	DraftInput
}

func (s *Service) CreatePackage(ctx context.Context, kind domain.Kind, in CreateInput, actor string) (domain.Package, error) {
	code, err := domain.NormalizeCode(in.Code)
	if err != nil {
		return domain.Package{}, err
	}
	now := s.now()
	draft := domain.Package{
		Kind: kind, Code: code, Version: 1, Status: domain.StatusDraft,
		Name: strings.TrimSpace(in.Name), Description: in.Description,
		MinAmount: in.MinAmount, MaxAmount: in.MaxAmount, Terms: in.Terms,
		CreatedBy: actor, CreatedAt: &now,
	}
	if err := draft.Validate(); err != nil {
		return domain.Package{}, err
	}
	// Index duy nhất theo mã và phiên bản chặn cả mã đã xóa và tạo đồng thời.
	if err := s.Store.InsertDraft(ctx, draft); err != nil {
		if errors.Is(err, ErrDraftExists) {
			return domain.Package{}, ErrCodeExists
		}
		return domain.Package{}, err
	}
	return draft, nil
}

// DeletePackage chỉ cho xóa gói không còn bán và chưa từng có hồ sơ hay hợp đồng;
// gói đã dùng phải giữ để tra cứu, nên chỉ có thể ngừng bán.
func (s *Service) DeletePackage(ctx context.Context, kind domain.Kind, code, actor string) error {
	versions, err := s.Store.Versions(ctx, kind, code)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return ErrPackageNotFound
	}
	for _, p := range versions {
		if p.Status == domain.StatusActive {
			return ErrPackageActive
		}
	}
	usage, err := s.Store.Usage(ctx, kind, code)
	if err != nil {
		return err
	}
	for _, count := range usage {
		if count > 0 {
			return ErrPackageInUse
		}
	}
	return s.Store.DeletePackage(ctx, kind, code, actor, s.now())
}

// CreateDraft sao chép bản mới nhất thành bản nháp.
func (s *Service) CreateDraft(ctx context.Context, kind domain.Kind, code, actor string) (domain.Package, error) {
	versions, err := s.Store.Versions(ctx, kind, code)
	if err != nil {
		return domain.Package{}, err
	}
	if len(versions) == 0 {
		return domain.Package{}, ErrPackageNotFound
	}
	if versions[0].Status == domain.StatusDraft {
		return domain.Package{}, ErrDraftExists
	}
	now := s.now()
	draft := versions[0]
	draft.Version++
	draft.Status = domain.StatusDraft
	draft.CreatedBy, draft.CreatedAt = actor, &now
	draft.PublishedBy, draft.PublishedAt = "", nil
	draft.RetiredBy, draft.RetiredAt, draft.RetireReason = "", nil, ""
	if err := s.Store.InsertDraft(ctx, draft); err != nil {
		return domain.Package{}, err
	}
	return draft, nil
}

// DraftInput là phần admin được sửa. Bản nháp chưa cần hợp lệ hoàn toàn nhưng
// không được ghi dữ liệu mà màn hình không đọc lại được.
type DraftInput struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	MinAmount   int64               `json:"minAmount"`
	MaxAmount   int64               `json:"maxAmount"`
	Terms       []domain.TermOption `json:"terms"`
}

func (s *Service) UpdateDraft(ctx context.Context, kind domain.Kind, code string, in DraftInput) (domain.Package, error) {
	versions, err := s.Store.Versions(ctx, kind, code)
	if err != nil {
		return domain.Package{}, err
	}
	if len(versions) == 0 {
		return domain.Package{}, ErrPackageNotFound
	}
	draft := versions[0]
	if draft.Status != domain.StatusDraft {
		return domain.Package{}, ErrNoDraft
	}
	draft.Name, draft.Description = strings.TrimSpace(in.Name), in.Description
	draft.MinAmount, draft.MaxAmount, draft.Terms = in.MinAmount, in.MaxAmount, in.Terms
	if err := draft.Validate(); err != nil {
		return domain.Package{}, err
	}
	if err := s.Store.ReplaceDraft(ctx, draft); err != nil {
		return domain.Package{}, err
	}
	return draft, nil
}

func (s *Service) DeleteDraft(ctx context.Context, kind domain.Kind, code string) error {
	return s.Store.DeleteDraft(ctx, kind, code)
}

func (s *Service) Publish(ctx context.Context, kind domain.Kind, code, actor, key string, expected int) (domain.Package, error) {
	if expected <= 0 || key == "" {
		return domain.Package{}, ErrInvalidRequest
	}
	versions, err := s.Store.Versions(ctx, kind, code)
	if err != nil {
		return domain.Package{}, err
	}
	// Validate trước; retry idempotent không còn bản nháp nên để Store trả kết quả cũ.
	for _, p := range versions {
		if p.Status == domain.StatusDraft && p.Version == expected {
			if err := p.Validate(); err != nil {
				return domain.Package{}, err
			}
		}
	}
	return s.Store.Publish(ctx, Transition{Kind: kind, Code: code, ExpectedVersion: expected, ActorID: actor, IdempotencyKey: key, At: s.now()})
}

func (s *Service) Retire(ctx context.Context, kind domain.Kind, code, actor, key, reason string, expected int) (domain.Package, error) {
	reason = strings.TrimSpace(reason)
	if expected <= 0 || key == "" || reason == "" || len([]rune(reason)) > 500 {
		return domain.Package{}, ErrInvalidRequest
	}
	return s.Store.Retire(ctx, Transition{Kind: kind, Code: code, ExpectedVersion: expected, ActorID: actor, Reason: reason, IdempotencyKey: key, At: s.now()})
}

func (s *Service) ListTerms(ctx context.Context) ([]TermsDocument, error) {
	return s.Store.ListTerms(ctx)
}

func (s *Service) SaveTermsDraft(ctx context.Context, in TermsDocument, actor string) (TermsDocument, error) {
	snapshot := savings.TermsSnapshot{Version: "draft", Title: in.Title, Introduction: in.Introduction, Sections: in.Sections}
	if err := snapshot.Validate(); err != nil {
		return TermsDocument{}, fmt.Errorf("%w: terms need a title, introduction and complete sections", domain.ErrInvalidPackage)
	}
	return s.Store.SaveTermsDraft(ctx, in, actor, s.now())
}

func (s *Service) DeleteTermsDraft(ctx context.Context) error {
	return s.Store.DeleteTermsDraft(ctx)
}

func (s *Service) PublishTerms(ctx context.Context, version, actor string) (TermsDocument, error) {
	if strings.TrimSpace(version) == "" {
		return TermsDocument{}, ErrInvalidRequest
	}
	return s.Store.PublishTerms(ctx, version, actor, s.now())
}

// LoanSimulation là kết quả mô phỏng theo chính sách đang chạy; không lưu gì.
type LoanSimulation struct {
	AnnualRateBps  int64                        `json:"annualRateBps"`
	MonthlyPayment int64                        `json:"monthlyPayment"`
	TotalInterest  int64                        `json:"totalInterest"`
	TotalPayment   int64                        `json:"totalPayment"`
	Schedule       []loan.RepaymentScheduleItem `json:"schedule"`
}

func (s *Service) SimulateLoan(amount, monthlyIncome int64, region string, term int) (LoanSimulation, error) {
	rate, err := loan.CalculateAnnualLoanRateBps(monthlyIncome, region, term)
	if err != nil {
		return LoanSimulation{}, errors.Join(ErrInvalidRequest, err)
	}
	schedule, err := loan.CalculateRepaymentSchedule(amount, rate, term, s.now())
	if err != nil {
		return LoanSimulation{}, errors.Join(ErrInvalidRequest, err)
	}
	out := LoanSimulation{AnnualRateBps: rate, Schedule: schedule}
	for _, item := range schedule {
		out.TotalInterest += item.InterestAmount
		out.TotalPayment += item.TotalPayment
		out.MonthlyPayment = max(out.MonthlyPayment, item.TotalPayment)
	}
	return out, nil
}

// SavingsSimulation dùng đúng công thức tất toán, tính từ thời điểm hiện tại.
type SavingsSimulation struct {
	Interest   int64     `json:"interest"`
	Total      int64     `json:"total"`
	MaturityAt time.Time `json:"maturityAt"`
}

func (s *Service) SimulateSavings(amount, rateBps int64, term int) (SavingsSimulation, error) {
	now := s.now()
	maturity, err := savings.MaturityForStoredContract(now, term)
	if err != nil {
		return SavingsSimulation{}, errors.Join(ErrInvalidRequest, err)
	}
	interest, total, err := savings.CalculateMaturityPayout(amount, rateBps, now, maturity)
	if err != nil {
		return SavingsSimulation{}, errors.Join(ErrInvalidRequest, err)
	}
	return SavingsSimulation{Interest: interest, Total: total, MaturityAt: maturity}, nil
}
