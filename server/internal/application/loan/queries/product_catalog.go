package queries

import (
	"context"
	"errors"
)

// LoanPackage chứa thông tin gói vay hiển thị cho khách hàng.
type LoanPackage struct {
	PackageCode string `json:"packageCode"`
	Name        string `json:"name"`
	MinAmount   int64  `json:"minAmount"`
	MaxAmount   int64  `json:"maxAmount"`
	TermMonths  []int  `json:"termMonths"`
}

// LoanPackageReader là Port đọc danh mục từ Read Model.
type LoanPackageReader interface {
	ListActiveLoanPackages(
		ctx context.Context,
	) ([]LoanPackage, error)
}

// LoanProductCatalogQueryService xử lý truy vấn danh mục gói vay.
type LoanProductCatalogQueryService struct {
	Reader LoanPackageReader
}

func NewLoanProductCatalogQueryService(
	reader LoanPackageReader,
) *LoanProductCatalogQueryService {
	return &LoanProductCatalogQueryService{
		Reader: reader,
	}
}

// List lấy các gói vay đang áp dụng.
// Không tính lãi suất cá nhân và không tạo hồ sơ vay.
func (s *LoanProductCatalogQueryService) List(
	ctx context.Context,
) ([]LoanPackage, error) {
	if s == nil || s.Reader == nil {
		return nil, errors.New(
			"loan package reader is not configured",
		)
	}

	items, err := s.Reader.ListActiveLoanPackages(ctx)
	if err != nil {
		return nil, err
	}

	if items == nil {
		return []LoanPackage{}, nil
	}

	return items, nil
}
