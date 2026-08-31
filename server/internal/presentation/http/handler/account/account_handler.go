package account

import (
	"errors"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type Handler struct {
	logger     zerolog.Logger
	register   *commands.RegisterAccountCommandHandler
	verify     *commands.VerifyAccountCommandHandler
	login      *queries.LoginQueryHandler
	getByID    *queries.GetAccountByIDQueryHandler
	getByEmail *queries.GetAccountByEmailQueryHandler
	getAll     *queries.GetAllAccountsQueryHandler
}

func NewHandler(
	logger zerolog.Logger,
	register *commands.RegisterAccountCommandHandler,
	verify *commands.VerifyAccountCommandHandler,
	login *queries.LoginQueryHandler,
	getByID *queries.GetAccountByIDQueryHandler,
	getByEmail *queries.GetAccountByEmailQueryHandler,
	getAll *queries.GetAllAccountsQueryHandler,
) *Handler {
	return &Handler{
		logger:     logger,
		register:   register,
		verify:     verify,
		login:      login,
		getByID:    getByID,
		getByEmail: getByEmail,
		getAll:     getAll,
	}
}

func accountErrorStatus(err error) int {
	switch {
	case errors.Is(err, accountdomain.ErrUserAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, accountdomain.ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, accountdomain.ErrAccountNotFound):
		return http.StatusNotFound
	case errors.Is(err, accountdomain.ErrInvalidEmail),
		errors.Is(err, accountdomain.ErrWeakPassword),
		errors.Is(err, accountdomain.ErrInvalidUsername),
		errors.Is(err, accountdomain.ErrInvalidAmount),
		errors.Is(err, accountdomain.ErrInsufficientFunds):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(err error) int {
			if errors.Is(err, httpx.ErrBodyTooLarge) {
				return http.StatusRequestEntityTooLarge
			}
			return http.StatusBadRequest
		})
		return
	}

	acc, err := h.register.Handle(r.Context(), commands.RegisterAccountCommand{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, toAccountResponse(acc))
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(err error) int {
			if errors.Is(err, httpx.ErrBodyTooLarge) {
				return http.StatusRequestEntityTooLarge
			}
			return http.StatusBadRequest
		})
		return
	}

	result, err := h.login.Handle(r.Context(), queries.LoginQuery{
		Login:    req.Login,
		Password: req.Password,
	})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toAuthResponse(result.User, result.Token, result.ExpiresAt))
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httpx.WriteError(w, h.logger, errors.New("missing claims"), func(err error) int {
			return http.StatusUnauthorized
		})
		return
	}

	acc, err := h.getByID.Handle(r.Context(), queries.GetAccountByIDQuery{ID: claims.UserID})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toAccountResponse(acc))
}

type verifyRequest struct {
	IdCardFrontUrl string `json:"idCardFrontUrl"`
	IdCardBackUrl  string `json:"idCardBackUrl"`
	LiveVideoUrl   string `json:"liveVideoUrl"`
}

func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httpx.WriteError(w, h.logger, errors.New("missing claims"), func(err error) int {
			return http.StatusUnauthorized
		})
		return
	}
	if claims.UserID != id {
		httpx.WriteError(w, h.logger, errors.New("user mismatch"), func(err error) int {
			return http.StatusForbidden
		})
		return
	}

	var req verifyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(err error) int {
			if errors.Is(err, httpx.ErrBodyTooLarge) {
				return http.StatusRequestEntityTooLarge
			}
			return http.StatusBadRequest
		})
		return
	}

	acc, err := h.verify.Handle(r.Context(), commands.VerifyAccountCommand{
		AccountID:      id,
		IdCardFrontUrl: req.IdCardFrontUrl,
		IdCardBackUrl:  req.IdCardBackUrl,
		LiveVideoUrl:   req.LiveVideoUrl,
	})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toAccountResponse(acc))
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	acc, err := h.getByID.Handle(r.Context(), queries.GetAccountByIDQuery{ID: id})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toAccountResponse(acc))
}

func (h *Handler) GetByEmail(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")

	acc, err := h.getByEmail.Handle(r.Context(), queries.GetAccountByEmailQuery{Email: email})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toAccountResponse(acc))
}

func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.getAll.Handle(r.Context(), queries.GetAllAccountsQuery{})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toAccountResponses(accounts))
}
