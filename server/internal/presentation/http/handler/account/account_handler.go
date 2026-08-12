package account

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

type Handler struct {
	create     *commands.CreateAccountCommandHandler
	getByID    *queries.GetAccountByIDQueryHandler
	getByEmail *queries.GetAccountByEmailQueryHandler
	getAll     *queries.GetAllAccountsQueryHandler
}

func NewHandler(
	create *commands.CreateAccountCommandHandler,
	getByID *queries.GetAccountByIDQueryHandler,
	getByEmail *queries.GetAccountByEmailQueryHandler,
	getAll *queries.GetAllAccountsQueryHandler,
) *Handler {
	return &Handler{
		create:     create,
		getByID:    getByID,
		getByEmail: getByEmail,
		getAll:     getAll,
	}
}

func accountErrorStatus(err error) int {
	switch {
	case errors.Is(err, accountdomain.ErrAccountNotFound):
		return http.StatusNotFound
	case errors.Is(err, accountdomain.ErrInvalidAmount),
		errors.Is(err, accountdomain.ErrInsufficientFunds):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

type createAccountRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, err, func(error) int { return http.StatusBadRequest })
		return
	}

	acc, err := h.create.Handle(r.Context(), commands.CreateAccountCommand{
		Username: req.Username,
		Email:    req.Email,
	})
	if err != nil {
		httpx.WriteError(w, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, acc)
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	acc, err := h.getByID.Handle(r.Context(), queries.GetAccountByIDQuery{ID: id})
	if err != nil {
		httpx.WriteError(w, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, acc)
}

func (h *Handler) GetByEmail(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")

	acc, err := h.getByEmail.Handle(r.Context(), queries.GetAccountByEmailQuery{Email: email})
	if err != nil {
		httpx.WriteError(w, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, acc)
}

func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.getAll.Handle(r.Context(), queries.GetAllAccountsQuery{})
	if err != nil {
		httpx.WriteError(w, err, accountErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, accounts)
}
