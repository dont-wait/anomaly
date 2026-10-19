package loan

import (
	"errors"
	"fmt"
	"time"
)

// Trang thai cua khoan vay da giai ngan.
// Khac voi ApplicationStatus cua ho so vay.
type LoanRepaymentStatus string

const (
	LoanRepaymentActive    LoanRepaymentStatus = "active"
	LoanRepaymentCompleted LoanRepaymentStatus = "completed"
)

// Trang thai cua tung ky thanh toan.
type LoanInstallmentStatus string

const (
	LoanInstallmentUpcoming LoanInstallmentStatus = "upcoming"
	LoanInstallmentPaid     LoanInstallmentStatus = "paid"
)

// Cac loi nghiep vu tra no.
var (
	ErrLoanNotDisbursed = errors.New(
		"loan has not been disbursed",
	)
	ErrInvalidRepaymentState = errors.New(
		"invalid loan repayment state",
	)
	ErrInvalidRepaymentDate = errors.New(
		"invalid repayment date",
	)
	ErrInstallmentNotDue = errors.New(
		"installment is not due today",
	)
	ErrInstallmentAlreadyPaid = errors.New(
		"installment has already been paid",
	)
	ErrInvalidInstallmentOrder = errors.New(
		"installment must be paid in order",
	)
	ErrLoanRepaymentCompleted = errors.New(
		"loan has already been fully repaid",
	)
)

// LoanRepayment quan ly tien do thanh toan cua khoan vay.
// Chi ho tro tra dung ngay, du so tien cua mot ky.
//
// So tien goc va lai lay tu lich EMI da chot khi giai ngan.
// Khong tinh lai suat moi khi khach hang tra no.
type LoanRepayment struct {
	LoanID               string
	ApplicationID        string
	CustomerID           string
	Principal            int64
	OutstandingPrincipal int64
	AnnualRateBps        int64
	TermMonths           int
	DisbursedAt          time.Time
	Status               LoanRepaymentStatus
	PaidInstallments     int
	Schedule             []RepaymentScheduleItem
}

// NewLoanRepaymentFromDisbursed khoi tao trang thai
// tra no tu mot ho so vay da duoc giai ngan.
//
// Giai doan nay su dung Application.Id lam LoanID
// de tuong thich voi Banking LoanID trong source.
//
// Phuc hoi trang thai tu EventStoreDB se duoc
// trien khai o giai doan Application/Write.
func NewLoanRepaymentFromDisbursed(
	a *Application,
) (*LoanRepayment, error) {
	if a == nil ||
		a.Status != ApplicationStatusDisbursed ||
		a.DisbursedAt == nil {
		return nil, ErrLoanNotDisbursed
	}

	if a.Id == "" ||
		a.CustomerId == "" ||
		a.RequestedAmount <= 0 ||
		a.AnnualRateBps <= 0 ||
		a.TermMonths <= 0 ||
		len(a.RepaymentSchedule) != a.TermMonths ||
		a.DisbursedAt.IsZero() {
		return nil, ErrInvalidRepaymentState
	}

	// Kiem tra tinh hop le cua lich tra no da chot.
	remaining := a.RequestedAmount

	for i, installment := range a.RepaymentSchedule {
		if installment.InstallmentNo != i+1 ||
			installment.DueDate.IsZero() ||
			installment.OpeningBalance != remaining ||
			installment.PrincipalAmount <= 0 ||
			installment.PrincipalAmount > remaining ||
			installment.InterestAmount < 0 ||
			installment.TotalPayment < installment.PrincipalAmount ||
			installment.TotalPayment-installment.PrincipalAmount != installment.InterestAmount ||
			installment.RemainingBalance != remaining-installment.PrincipalAmount {

			return nil, fmt.Errorf(
				"%w: installment %d",
				ErrInvalidRepaymentState,
				i+1,
			)
		}

		remaining = installment.RemainingBalance
	}

	// Sau ky cuoi, du no phai bang 0.
	if remaining != 0 {
		return nil, ErrInvalidRepaymentState
	}

	return &LoanRepayment{
		LoanID:               a.Id,
		ApplicationID:        a.Id,
		CustomerID:           a.CustomerId,
		Principal:            a.RequestedAmount,
		OutstandingPrincipal: a.RequestedAmount,
		AnnualRateBps:        a.AnnualRateBps,
		TermMonths:           a.TermMonths,
		DisbursedAt:          *a.DisbursedAt,
		Status:               LoanRepaymentActive,
		Schedule: append(
			[]RepaymentScheduleItem(nil),
			a.RepaymentSchedule...,
		),
	}, nil
}

// DueInstallment tra ve ky thanh toan tiep theo
// neu ngay hien tai trung voi ngay den han.
//
// So sanh theo ngay Viet Nam (UTC+7).
// DueDate duoc luu theo ngay UTC tu lich EMI.
func (r *LoanRepayment) DueInstallment(
	at time.Time,
) (RepaymentScheduleItem, error) {
	if r == nil {
		return RepaymentScheduleItem{},
			ErrInvalidRepaymentState
	}

	if r.Status == LoanRepaymentCompleted {
		return RepaymentScheduleItem{},
			ErrLoanRepaymentCompleted
	}

	if r.Status != LoanRepaymentActive ||
		r.TermMonths <= 0 ||
		len(r.Schedule) != r.TermMonths ||
		r.PaidInstallments < 0 ||
		r.PaidInstallments >= r.TermMonths {

		return RepaymentScheduleItem{},
			ErrInvalidRepaymentState
	}

	if at.IsZero() {
		return RepaymentScheduleItem{},
			ErrInvalidRepaymentDate
	}

	item := r.Schedule[r.PaidInstallments]

	if item.InstallmentNo != r.PaidInstallments+1 ||
		item.OpeningBalance != r.OutstandingPrincipal {

		return RepaymentScheduleItem{},
			ErrInvalidRepaymentState
	}

	vn := time.FixedZone("ICT", 7*60*60)

	year, month, day := at.In(vn).Date()
	dueYear, dueMonth, dueDay := item.DueDate.UTC().Date()

	// Chi cho phep thanh toan dung ngay den han.
	if year != dueYear ||
		month != dueMonth ||
		day != dueDay {

		return RepaymentScheduleItem{},
			ErrInstallmentNotDue
	}

	return item, nil
}

// RecordInstallmentPayment ghi nhan mot ky da thanh toan.
//
// CHI GOI SAU KHI Banking ghi no thanh cong.
// Giai doan Domain chi cap nhat trang thai trong memory.
//
// Application layer se chiu trach nhiem:
// - Tru tien tai khoan.
// - Ghi EventStoreDB.
// - Chong thanh toan trung.
// - Khoi phuc khi co loi.
func (r *LoanRepayment) RecordInstallmentPayment(
	installmentNo int,
	paidAt time.Time,
) (RepaymentScheduleItem, error) {
	if r == nil {
		return RepaymentScheduleItem{},
			ErrInvalidRepaymentState
	}

	// Ngan thanh toan trung ky da tra.
	if installmentNo > 0 &&
		installmentNo <= r.PaidInstallments {

		return RepaymentScheduleItem{},
			ErrInstallmentAlreadyPaid
	}

	if r.Status == LoanRepaymentCompleted {
		return RepaymentScheduleItem{},
			ErrLoanRepaymentCompleted
	}

	// Phai tra theo thu tu tung ky.
	if installmentNo != r.PaidInstallments+1 {
		return RepaymentScheduleItem{},
			ErrInvalidInstallmentOrder
	}

	item, err := r.DueInstallment(paidAt)
	if err != nil {
		return RepaymentScheduleItem{}, err
	}

	// Ky cuoi phai tra het du no.
	if r.PaidInstallments+1 == r.TermMonths &&
		item.RemainingBalance != 0 {

		return RepaymentScheduleItem{},
			ErrInvalidRepaymentState
	}

	// Cap nhat du no va so ky da tra.
	r.OutstandingPrincipal = item.RemainingBalance
	r.PaidInstallments++

	// Hoan thanh khoan vay sau ky cuoi.
	if r.PaidInstallments == r.TermMonths {
		r.Status = LoanRepaymentCompleted
	}

	return item, nil
}

func (r *LoanRepayment) InstallmentStatus(
	installmentNo int,
) (LoanInstallmentStatus, error) {
	if r == nil ||
		installmentNo <= 0 ||
		installmentNo > r.TermMonths {

		return "", ErrInvalidRepaymentState
	}

	if installmentNo <= r.PaidInstallments {
		return LoanInstallmentPaid, nil
	}

	return LoanInstallmentUpcoming, nil
}
