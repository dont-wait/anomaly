// Package catalog mô tả danh mục gói vay và tiết kiệm. Gói là cấu hình để bán sản
// phẩm mới, không phải sổ cái: hợp đồng đã mở giữ điều khoản riêng trong sự kiện.
package catalog

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
)

type Kind string

const (
	KindLoan    Kind = "loan"
	KindSavings Kind = "savings"
)

type Status string

const (
	StatusDraft      Status = "DRAFT"
	StatusActive     Status = "ACTIVE"
	StatusSuperseded Status = "SUPERSEDED"
	StatusRetired    Status = "RETIRED"
)

var ErrInvalidPackage = errors.New("invalid package")

// loanTerms là các kỳ hạn mà ma trận lãi suất vay tính được.
var loanTerms = []int{6, 12, 24, 36, 48, 60}

const (
	maxNameLength        = 120
	maxDescriptionLength = 1000
	// Trùng giới hạn lãi khi replay sự kiện tiết kiệm.
	maxAnnualRateBps = 10_000
)

type TermOption struct {
	Months int `json:"months"`
	// AnnualRateBps chỉ dùng cho tiết kiệm; lãi vay tính theo hồ sơ.
	AnnualRateBps int64 `json:"annualRateBps,omitempty"`
}

type Package struct {
	Kind         Kind         `json:"kind"`
	Code         string       `json:"code"`
	Version      int          `json:"version"`
	Status       Status       `json:"status"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	MinAmount    int64        `json:"minAmount"`
	MaxAmount    int64        `json:"maxAmount,omitempty"`
	Terms        []TermOption `json:"terms"`
	CreatedBy    string       `json:"createdBy,omitempty"`
	CreatedAt    *time.Time   `json:"createdAt,omitempty"`
	PublishedBy  string       `json:"publishedBy,omitempty"`
	PublishedAt  *time.Time   `json:"publishedAt,omitempty"`
	RetiredBy    string       `json:"retiredBy,omitempty"`
	RetiredAt    *time.Time   `json:"retiredAt,omitempty"`
	RetireReason string       `json:"retireReason,omitempty"`
}

var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{2,63}$`)

// NormalizeCode chuẩn hóa mã gói mới về chữ hoa để không có hai mã chỉ khác hoa thường.
func NormalizeCode(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !codePattern.MatchString(code) {
		return "", invalid("code must be 3-64 letters, digits, dashes or underscores")
	}
	return code, nil
}

func invalid(message string) error {
	return errors.Join(ErrInvalidPackage, errors.New(message))
}

// Validate kiểm tra điều kiện phát hành.
func (p Package) Validate() error {
	name := strings.TrimSpace(p.Name)
	if name == "" || len([]rune(name)) > maxNameLength {
		return invalid("name is required and at most 120 characters")
	}
	if len([]rune(p.Description)) > maxDescriptionLength {
		return invalid("description is at most 1000 characters")
	}
	if len(p.Terms) == 0 {
		return invalid("at least one term is required")
	}
	seen := map[int]bool{}
	for _, term := range p.Terms {
		if seen[term.Months] {
			return invalid("terms must not repeat")
		}
		seen[term.Months] = true
	}
	switch p.Kind {
	case KindLoan:
		return p.validateLoan()
	case KindSavings:
		return p.validateSavings()
	}
	return invalid("unknown package kind")
}

func (p Package) validateLoan() error {
	if p.MinAmount <= 0 || p.MaxAmount < p.MinAmount {
		return invalid("amount limits must satisfy 0 < min <= max")
	}
	for _, term := range p.Terms {
		if !slices.Contains(loanTerms, term.Months) {
			return invalid("unsupported loan term")
		}
		if term.AnnualRateBps != 0 {
			return invalid("loan rates are calculated per application, not per package")
		}
	}
	return nil
}

func (p Package) validateSavings() error {
	// Hằng số là sàn cố định vì replay sự kiện cũ phụ thuộc vào nó.
	if p.MinAmount < savings.MinimumDepositVND {
		return invalid("minimum deposit is below the 1,000,000 VND floor")
	}
	if p.MaxAmount != 0 {
		return invalid("savings packages have no maximum deposit")
	}
	for _, term := range p.Terms {
		if !savings.IsSupportedTerm(term.Months) {
			return invalid("unsupported savings term")
		}
		if term.AnnualRateBps <= 0 || term.AnnualRateBps > maxAnnualRateBps {
			return invalid("annual rate must be above 0 and at most 100 percent")
		}
	}
	return nil
}
