package loan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"

	accountqueries "github.com/dont-wait/anomaly/internal/application/account/queries"
	commands "github.com/dont-wait/anomaly/internal/application/loan/commands"
	domain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	"github.com/google/uuid"
)

const maxLoanDocSize int64 = 10 << 20

type loanListCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`
}

type loanPageReader interface {
	ListByCustomerPage(
		context.Context,
		string,
		int,
		time.Time,
		string,
	) ([]*domain.Application, bool, error)

	ListAllPage(
		context.Context,
		int,
		time.Time,
		string,
	) ([]*domain.Application, bool, error)
}

type Reader interface {
	FindByID(
		context.Context,
		string,
	) (*domain.Application, error)
}

type ListReader interface {
	ListByCustomer(
		context.Context,
		string,
		int,
	) ([]*domain.Application, error)

	ListAll(
		context.Context,
		int,
	) ([]*domain.Application, error)
}

type Evidence interface {
	Save(
		context.Context,
		domain.VerifiedEvidence,
	) error

	Find(
		context.Context,
		string,
	) (domain.VerifiedEvidence, error)
}

type Media interface {
	Upload(
		context.Context,
		string,
		io.Reader,
		string,
	) error

	Download(
		context.Context,
		string,
	) (*rustfs.DownloadResult, error)

	Delete(
		context.Context,
		string,
	) error
}

type loanQuoteResponse struct {
	AnnualRateBps       int64                          `json:"annualRateBps"`
	MonthlyIncome       int64                          `json:"monthlyIncome"`
	Affordability       *domain.LoanAffordability      `json:"affordability"`
	ProvisionalSchedule []domain.RepaymentScheduleItem `json:"provisionalSchedule"`
	Official            bool                           `json:"official"`
}

type Handler struct {
	Create   *commands.CreateLoanApplicationCommandHandler
	Add      *commands.AddLoanDocumentCommandHandler
	Submit   *commands.SubmitLoanApplicationCommandHandler
	Review   *commands.StartLoanReviewCommandHandler
	Assess   *commands.AssessLoanApplicationCommandHandler
	Decide   *commands.ManualLoanDecisionCommandHandler
	Disburse *commands.DisburseLoanCommandHandler

	Read     Reader
	List     ListReader
	Evidence Evidence
	Media    Media
	Reviewer commands.LoanReviewAuthorizer
}

func isLoanDocumentNotFound(err error) bool {
	if err == nil {
		return false
	}

	// MediaRepository thay lỗi S3 bằng sentinel; lỗi gốc không còn trong chain.
	if errors.Is(err, rustfs.ErrObjectNotFound) {
		return true
	}

	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		}
	}

	return false
}

func RegisterRoutes(
	router *openapi.Registry,
	h *Handler,
	tokens accountqueries.TokenService,
) {
	if h == nil {
		return
	}

	auth := func(
		pattern string,
		handler http.HandlerFunc,
		methodID string,
	) {
		op := openapi.Operation{
			ID:   methodID,
			Tags: []string{"Loans"},
			Auth: true,
			FailureStatuses: []int{
				400, 401, 403, 404, 409, 413, 422, 500, 503,
			},
			Response: domain.Application{},
		}

		switch methodID {
		case "createPersonalLoan":
			op.SuccessStatus = 201
			op.Parameters = []openapi.Parameter{
				{
					Name:     "Idempotency-Key",
					In:       "header",
					Type:     "string",
					Required: true,
					Format:   "uuid",
				},
			}
			op.Request = struct {
				LoanPackageID         string `json:"loanPackageId"`
				DisbursementAccountID string `json:"disbursementAccountId"`
				RequestedAmount       int64  `json:"requestedAmount"`
				TermMonths            int    `json:"termMonths"`
			}{}

		case "uploadPersonalLoanDocument":
			op.SuccessStatus = 201
			op.RequestContentType = "multipart/form-data"
			op.Request = struct {
				Type string `json:"type"`
				File string `json:"file" format:"binary"`
			}{}
			op.Response = domain.ApplicationDocument{}

		case "attestPersonalLoanEvidence":
			op.SuccessStatus = 201
			op.Response = domain.VerifiedEvidence{}
			op.Description = "Admin verifies evidence in simulation"

		case "assessPersonalLoan":
			op.Description = "Requires all evidence verification flags to be true. Incomplete evidence returns 422 without recording an assessment; complete evidence via the evidence endpoint before retrying."

		case "listPersonalLoans":
			op.Response = struct {
				Items               []domain.Application `json:"items"`
				EventualConsistency bool                 `json:"eventualConsistency"`
			}{}

		case "downloadPersonalLoanDocument":
			op.Response = openapi.Binary{}
			op.ResponseContentType = "application/octet-stream"

		case "quotePersonalLoan":
			op.Description = "Provisional loan repayment schedule"
			op.Response = loanQuoteResponse{}

		case "disbursePersonalLoan":
			op.Description = "Admin-only loan disbursement"
		}

		protected := middleware.RequireAuth(tokens)(
			http.HandlerFunc(func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.Header().Set("Cache-Control", "no-store")
				handler(w, r)
			}),
		)

		router.Handle(pattern, protected, op)
	}

	auth(
		"POST /api/loans",
		h.create,
		"createPersonalLoan",
	)
	auth(
		"GET /api/loans",
		h.list,
		"listPersonalLoans",
	)
	auth(
		"GET /api/loans/{applicationId}",
		h.get,
		"getPersonalLoan",
	)
	auth(
		"POST /api/loans/{applicationId}/documents",
		h.upload,
		"uploadPersonalLoanDocument",
	)
	auth(
		"GET /api/loans/{applicationId}/documents/{documentId}",
		h.download,
		"downloadPersonalLoanDocument",
	)
	auth(
		"POST /api/loans/{applicationId}/submit",
		h.submit,
		"submitPersonalLoan",
	)
	auth(
		"POST /api/loans/{applicationId}/review",
		h.review,
		"startPersonalLoanReview",
	)
	auth(
		"POST /api/loans/{applicationId}/evidence",
		h.attest,
		"attestPersonalLoanEvidence",
	)
	auth(
		"POST /api/loans/{applicationId}/assess",
		h.assess,
		"assessPersonalLoan",
	)
	auth(
		"POST /api/loans/{applicationId}/decision",
		h.decide,
		"decidePersonalLoan",
	)
	auth(
		"GET /api/loans/{applicationId}/quote",
		h.quote,
		"quotePersonalLoan",
	)
	auth(
		"POST /api/loans/{applicationId}/disburse",
		h.disburse,
		"disbursePersonalLoan",
	)
}

func claims(r *http.Request) (string, string, bool) {
	c, ok := middleware.ClaimsFromContext(r.Context())

	if !ok || c == nil || c.UserID == "" {
		return "", "", false
	}

	return c.UserID, c.Role, true
}

func fail(
	w http.ResponseWriter,
	status int,
	message string,
) {
	if status >= 500 {
		message = "loan service is temporarily unavailable"
	}

	httpx.WriteJSON(
		w,
		status,
		map[string]any{
			"error": message,
		},
	)
}

func serviceError(
	w http.ResponseWriter,
	err error,
) {
	if err == nil {
		return
	}

	msg := err.Error()
	status := http.StatusUnprocessableEntity

	switch {
	case strings.Contains(msg, "not found"),
		strings.Contains(msg, "does not exist"):
		status = http.StatusNotFound

	case strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "permission"),
		strings.Contains(msg, "requires active admin"):
		status = http.StatusForbidden

	case strings.Contains(msg, "revision conflict"),
		strings.Contains(msg, "modified concurrently"),
		strings.Contains(msg, "already"),
		strings.Contains(msg, "idempotency key reused"):
		status = http.StatusConflict

	case strings.Contains(msg, "unavailable"),
		strings.Contains(msg, "failed"),
		strings.Contains(msg, "timeout"):
		status = http.StatusServiceUnavailable
	}

	fail(w, status, msg)
}

// Đọc hồ sơ và kiểm tra quyền truy cập.
func (h *Handler) application(
	r *http.Request,
	ownerOnly bool,
) (*domain.Application, error) {
	id := r.PathValue("applicationId")

	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("invalid application ID")
	}

	a, err := h.Read.FindByID(
		r.Context(),
		id,
	)
	if err != nil {
		return nil, err
	}

	if a == nil {
		return nil, fmt.Errorf("loan application not found")
	}

	actor, role, ok := claims(r)
	if !ok {
		return nil, fmt.Errorf("unauthorized")
	}

	if a.CustomerId != actor {
		if ownerOnly || role != "admin" {
			return nil, fmt.Errorf("loan application not found")
		}

		if err := h.admin(r); err != nil {
			return nil, err
		}
	}

	return a, nil
}

func (h *Handler) admin(r *http.Request) error {
	actor, role, ok := claims(r)

	if !ok || role != "admin" {
		return fmt.Errorf("unauthorized")
	}

	return h.Reviewer.AuthorizeLoanReview(
		r.Context(),
		actor,
	)
}

// POST /api/loans
func (h *Handler) create(
	w http.ResponseWriter,
	r *http.Request,
) {
	actor, _, ok := claims(r)
	if !ok {
		fail(w, 401, "unauthorized")
		return
	}

	key := r.Header.Get("Idempotency-Key")

	if _, err := uuid.Parse(key); err != nil {
		fail(w, 400, "Idempotency-Key must be a UUID")
		return
	}

	var req struct {
		LoanPackageID         string `json:"loanPackageId"`
		DisbursementAccountID string `json:"disbursementAccountId"`
		RequestedAmount       int64  `json:"requestedAmount"`
		TermMonths            int    `json:"termMonths"`
	}

	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		fail(w, 400, "invalid loan JSON")
		return
	}

	if req.LoanPackageID != "PERSONAL_UNSECURED_SALARY" {
		fail(w, 422, "unsupported loan package")
		return
	}

	a, err := h.Create.Handle(
		r.Context(),
		commands.CreateLoanApplicationCommand{
			CustomerID:            actor,
			LoanPackageID:         req.LoanPackageID,
			DisbursementAccountID: req.DisbursementAccountID,
			RequestedAmount:       req.RequestedAmount,
			TermMonths:            req.TermMonths,
			IdempotencyKey:        key,
		},
	)
	if err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, a)
}

// GET /api/loans/{applicationId}
func (h *Handler) get(
	w http.ResponseWriter,
	r *http.Request,
) {
	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, a)
}

// GET /api/loans

func (h *Handler) list(
	w http.ResponseWriter,
	r *http.Request,
) {
	actor, role, ok := claims(r)
	if !ok {
		fail(w, 401, "unauthorized")
		return
	}

	query := r.URL.Query()

	limit := 50
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			fail(w, 400, "limit must be between 1 and 100")
			return
		}
		limit = value
	}

	adminAll := role == "admin" && query.Get("all") == "true"

	if adminAll {
		if err := h.admin(r); err != nil {
			serviceError(w, err)
			return
		}
	}

	scope := "owner:" + actor
	if adminAll {
		scope = "all"
	}

	var after loanListCursor

	if token := query.Get("cursor"); token != "" {
		if len(token) > 2048 {
			fail(w, 400, "invalid pagination cursor")
			return
		}

		data, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			fail(w, 400, "invalid pagination cursor")
			return
		}

		if err := json.Unmarshal(data, &after); err != nil ||
			after.CreatedAt.IsZero() ||
			after.ID == "" ||
			after.Scope != scope {
			fail(w, 400, "invalid pagination cursor")
			return
		}
	}

	pager, ok := any(h.List).(loanPageReader)
	if !ok {
		fail(w, 500, "loan pagination service unavailable")
		return
	}

	var (
		items   []*domain.Application
		hasMore bool
		err     error
	)

	if adminAll {
		items, hasMore, err = pager.ListAllPage(
			r.Context(),
			limit,
			after.CreatedAt,
			after.ID,
		)
	} else {
		items, hasMore, err = pager.ListByCustomerPage(
			r.Context(),
			actor,
			limit,
			after.CreatedAt,
			after.ID,
		)
	}

	if err != nil {
		serviceError(w, err)
		return
	}

	if items == nil {
		items = []*domain.Application{}
	}

	var nextCursor any

	if hasMore && len(items) > 0 {
		last := items[len(items)-1]

		tokenData, err := json.Marshal(loanListCursor{
			CreatedAt: last.CreatedAt.UTC().Truncate(time.Millisecond),
			ID:        last.Id,
			Scope:     scope,
		})
		if err != nil {
			serviceError(w, err)
			return
		}

		nextCursor = base64.RawURLEncoding.EncodeToString(tokenData)
	}

	httpx.WriteJSON(
		w,
		200,
		map[string]any{
			"items":               items,
			"nextCursor":          nextCursor,
			"eventualConsistency": true,
		},
	)
}

func validDocumentType(t string) bool {
	switch t {
	case "IDENTITY_FRONT",
		"IDENTITY_BACK",
		"VNEID_PERSONAL",
		"VNEID_RESIDENCE",
		"VSSID_PARTICIPATION",
		"VSSID_DETAIL",
		"SALARY_STATEMENT_3_MONTHS":
		return true
	}

	return false
}

func validDocBytes(data []byte) string {
	if len(data) >= 5 &&
		bytes.Equal(data[:5], []byte("%PDF-")) {
		return "application/pdf"
	}

	detected := http.DetectContentType(data)

	switch detected {
	case "image/jpeg", "image/png":
		return detected
	}

	return ""
}

// POST /api/loans/{applicationId}/documents
func (h *Handler) upload(
	w http.ResponseWriter,
	r *http.Request,
) {
	a, err := h.application(r, true)
	if err != nil {
		serviceError(w, err)
		return
	}

	if a.Status != domain.ApplicationStatusDraft {
		fail(w, 409, "loan is not draft")
		return
	}

	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		maxLoanDocSize+(1<<20),
	)

	if err := r.ParseMultipartForm(1 << 20); err != nil {
		fail(w, 413, "loan upload exceeds limit")
		return
	}

	docType := r.FormValue("type")

	if !validDocumentType(docType) {
		fail(w, 422, "invalid loan document type")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "file is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(
		io.LimitReader(file, maxLoanDocSize+1),
	)
	if err != nil ||
		len(data) == 0 ||
		int64(len(data)) > maxLoanDocSize {
		fail(w, 413, "invalid loan document size")
		return
	}

	contentType := validDocBytes(data)

	if contentType == "" {
		fail(w, 422, "only PDF, PNG or JPEG supported")
		return
	}

	docID := uuid.NewString()

	key := "loans/" +
		a.CustomerId + "/" +
		a.Id + "/" +
		docID

	hash := sha256.Sum256(data)

	if err := h.Media.Upload(
		r.Context(),
		key,
		bytes.NewReader(data),
		contentType,
	); err != nil {
		fail(w, 503, "storage unavailable")
		return
	}

	saved, err := h.Add.Handle(
		r.Context(),
		commands.AddLoanDocumentCommand{
			ApplicationID: a.Id,
			CustomerID:    a.CustomerId,
			Document: domain.ApplicationDocument{
				DocumentId: docID,
				Type:       docType,
				StorageKey: key,
				MIMEType:   contentType,
				SizeBytes:  int64(len(data)),
				SHA256:     hex.EncodeToString(hash[:]),
			},
		},
	)
	if err != nil {
		// Không xóa file ngay nếu kết quả
		// Append event chưa chắc chắn.
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(
		w,
		201,
		saved.Documents[len(saved.Documents)-1],
	)
}

// GET /api/loans/{applicationId}/documents/{documentId}
func (h *Handler) download(
	w http.ResponseWriter,
	r *http.Request,
) {
	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}

	var document *domain.ApplicationDocument

	for i := range a.Documents {
		if a.Documents[i].DocumentId ==
			r.PathValue("documentId") {
			document = &a.Documents[i]
			break
		}
	}

	if document == nil {
		fail(w, 404, "document not found")
		return
	}

	res, err := h.Media.Download(
		r.Context(),
		document.StorageKey,
	)
	if err != nil {
		if isLoanDocumentNotFound(err) {
			fail(w, 404, "document not found")
		} else {
			fail(w, 503, "document storage temporarily unavailable")
		}
		return
	}
	defer res.Body.Close()

	w.Header().Set(
		"Content-Type",
		"application/octet-stream",
	)
	w.Header().Set(
		"X-Content-Type-Options",
		"nosniff",
	)
	w.Header().Set(
		"Content-Disposition",
		mime.FormatMediaType(
			"attachment",
			map[string]string{
				"filename": path.Base(document.StorageKey),
			},
		),
	)

	w.WriteHeader(200)
	_, _ = io.Copy(w, res.Body)
}

// POST /api/loans/{applicationId}/submit
func (h *Handler) submit(
	w http.ResponseWriter,
	r *http.Request,
) {
	a, err := h.application(r, true)
	if err != nil {
		serviceError(w, err)
		return
	}

	result, err := h.Submit.Handle(
		r.Context(),
		commands.SubmitLoanApplicationCommand{
			ApplicationID: a.Id,
			CustomerID:    a.CustomerId,
		},
	)
	if err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, result)
}

// POST /api/loans/{applicationId}/review
func (h *Handler) review(
	w http.ResponseWriter,
	r *http.Request,
) {
	if err := h.admin(r); err != nil {
		serviceError(w, err)
		return
	}

	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}

	actor, _, _ := claims(r)

	result, err := h.Review.Handle(
		r.Context(),
		commands.StartLoanReviewCommand{
			ApplicationID: a.Id,
			ActorID:       actor,
		},
	)
	if err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, result)
}

// POST /api/loans/{applicationId}/evidence
func (h *Handler) attest(
	w http.ResponseWriter,
	r *http.Request,
) {
	if err := h.admin(r); err != nil {
		serviceError(w, err)
		return
	}

	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}

	if a.Status != domain.ApplicationStatusUnderReview ||
		a.AssessedAt != nil {
		fail(w, 409, "loan is not awaiting evidence")
		return
	}

	var req struct {
		MonthlySalaries       [3]int64                      `json:"monthlySalaries"`
		RegionCode            string                        `json:"regionCode"`
		ExistingMonthlyDebt   int64                         `json:"existingMonthlyDebt"`
		InsuranceMonths       int                           `json:"insuranceMonths"`
		InsuranceVerified     bool                          `json:"insuranceVerified"`
		RepaymentHistory      domain.RepaymentHistoryStatus `json:"repaymentHistory"`
		HistoryVerified       bool                          `json:"historyVerified"`
		FinancialDataVerified bool                          `json:"financialDataVerified"`
		DocumentsVerified     bool                          `json:"documentsVerified"`
		Sources               []string                      `json:"sources"`
	}

	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		fail(w, 400, "invalid evidence JSON")
		return
	}

	actor, _, _ := claims(r)

	e := domain.VerifiedEvidence{
		ApplicationID:         a.Id,
		CustomerID:            a.CustomerId,
		SalaryAccountID:       a.DisbursementAccountId,
		MonthlySalaries:       req.MonthlySalaries,
		RegionCode:            req.RegionCode,
		ExistingMonthlyDebt:   req.ExistingMonthlyDebt,
		InsuranceMonths:       req.InsuranceMonths,
		InsuranceVerified:     req.InsuranceVerified,
		RepaymentHistory:      req.RepaymentHistory,
		HistoryVerified:       req.HistoryVerified,
		FinancialDataVerified: req.FinancialDataVerified,
		DocumentsVerified:     req.DocumentsVerified,
		Sources:               req.Sources,
		VerifiedBy:            actor,
		VerifiedAt:            time.Now().UTC(),
	}

	if err := e.ValidateFor(a); err != nil {
		serviceError(w, err)
		return
	}

	if err := h.Evidence.Save(
		r.Context(),
		e,
	); err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(w, 201, e)
}

// POST /api/loans/{applicationId}/assess
func (h *Handler) assess(
	w http.ResponseWriter,
	r *http.Request,
) {
	if err := h.admin(r); err != nil {
		serviceError(w, err)
		return
	}

	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}

	actor, _, _ := claims(r)

	result, err := h.Assess.Handle(
		r.Context(),
		commands.AssessLoanApplicationCommand{
			ApplicationID: a.Id,
			ActorID:       actor,
		},
	)
	if err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, result)
}

// POST /api/loans/{applicationId}/decision
func (h *Handler) decide(
	w http.ResponseWriter,
	r *http.Request,
) {
	if err := h.admin(r); err != nil {
		serviceError(w, err)
		return
	}

	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}

	var req struct {
		Decision domain.ApplicationStatus `json:"decision"`
		Reason   string                   `json:"reason"`
	}

	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		fail(w, 400, "invalid decision JSON")
		return
	}

	if req.Decision == domain.ApplicationStatusApproved {

		e, err := h.Evidence.Find(
			r.Context(),
			a.Id,
		)
		if err != nil {
			if errors.Is(err, mongodrv.ErrNoDocuments) {
				fail(w, 404, "loan evidence not found")
			} else {
				fail(w, 503, "loan evidence service unavailable")
			}
			return
		}

		if !e.FinancialDataVerified ||
			!e.DocumentsVerified ||
			!e.InsuranceVerified ||
			!e.HistoryVerified {
			fail(
				w,
				422,
				"manual approval requires complete verified evidence",
			)
			return
		}

	}

	actor, _, _ := claims(r)

	result, err := h.Decide.Handle(
		r.Context(),
		commands.ManualLoanDecisionCommand{
			ApplicationID: a.Id,
			ActorID:       actor,
			Decision:      req.Decision,
			Reason:        req.Reason,
		},
	)
	if err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, result)
}

// GET /api/loans/{applicationId}/quote
func (h *Handler) quote(
	w http.ResponseWriter,
	r *http.Request,
) {
	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}

	e, err := h.Evidence.Find(
		r.Context(),
		a.Id,
	)
	if err != nil {
		if errors.Is(err, mongodrv.ErrNoDocuments) {
			fail(w, 404, "loan evidence not available")
		} else {
			fail(w, 503, "loan evidence service unavailable")
		}
		return
	}

	input, rate, err := e.BuildAssessment(
		a,
		time.Now().UTC(),
	)
	if err != nil {
		serviceError(w, err)
		return
	}

	schedule, err := domain.CalculateRepaymentSchedule(
		a.RequestedAmount,
		rate,
		a.TermMonths,
		time.Now().UTC(),
	)
	if err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(
		w,
		http.StatusOK,
		loanQuoteResponse{
			AnnualRateBps:       rate,
			MonthlyIncome:       input.MonthlyIncome,
			Affordability:       input.Affordability,
			ProvisionalSchedule: schedule,
			Official:            false,
		},
	)
}

// POST /api/loans/{applicationId}/disburse
func (h *Handler) disburse(
	w http.ResponseWriter,
	r *http.Request,
) {
	if err := h.admin(r); err != nil {
		serviceError(w, err)
		return
	}

	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}

	actor, _, _ := claims(r)

	result, err := h.Disburse.Handle(
		r.Context(),
		commands.DisburseLoanCommand{
			ApplicationID: a.Id,
			ActorID:       actor,
		},
	)
	if err != nil {
		serviceError(w, err)
		return
	}

	httpx.WriteJSON(w, 200, result)
}
