package media_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/presentation/http/handler/media"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

type accountRepo struct{ account *accountdomain.UserAccount }

func (r accountRepo) FindByID(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, nil
}
func (accountRepo) FindByEmail(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}
func (accountRepo) FindByUsername(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}
func (accountRepo) FindByCCCDNumber(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}
func (accountRepo) FindAll(context.Context) ([]*accountdomain.UserAccount, error) { return nil, nil }

func TestRemovedMediaUploadRoutesReturnNotFound(t *testing.T) {
	tokens := auth.NewTokenService("test-secret", time.Hour)
	mux := http.NewServeMux()
	media.RegisterRoutes(openapi.NewRegistry(mux, false), media.NewHandler(zerolog.Nop(), nil), tokens)
	for _, path := range []string{"/api/media/upload", "/api/kyc/media"} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader("ignored")))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("POST %s status = %d, want 404", path, recorder.Code)
		}
	}
}

func TestDownloadDefaultsToDenyForUnknownAndOtherOwnerKeys(t *testing.T) {
	account := &accountdomain.UserAccount{Id: "owner", Customer: &accountdomain.Customer{KYCStatus: accountdomain.KYCStatusVerified}}
	tokens := auth.NewTokenService("test-secret", time.Hour)
	token, _, err := tokens.Issue("owner", "owner", "admin", true)
	if err != nil {
		t.Fatal(err)
	}
	handler := media.NewHandler(zerolog.Nop(), nil, accountRepo{account: account})
	mux := http.NewServeMux()
	media.RegisterRoutes(openapi.NewRegistry(mux, false), handler, tokens)
	for _, key := range []string{"public/file", "kyc/other/id-card-front/file.jpg", "kyc/owner/unknown/file.jpg"} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/media/download?key="+key, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		mux.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("key %q status = %d, want 403", key, recorder.Code)
		}
	}
}

func TestOwnerDownloadUsesSafeResponseHeaders(t *testing.T) {
	key := "kyc/owner/id-card-front/front.jpg"
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg"))
	}))
	defer storage.Close()
	client := rustfs.NewClient(&domain.RustFSConfig{Endpoint: storage.URL, AccessKey: "access", SecretKey: "secret", Bucket: "media", Region: "us-east-1"})
	account := &accountdomain.UserAccount{
		Id: "owner", Customer: &accountdomain.Customer{KYCStatus: accountdomain.KYCStatusVerified, VerifiedKYCSessionId: "session"},
		KYCSessions: []*accountdomain.KYCSession{{Id: "session", Media: accountdomain.KYCMedia{IdentityFront: accountdomain.MediaObject{StorageKey: key}}}},
	}
	tokens := auth.NewTokenService("test-secret", time.Hour)
	token, _, err := tokens.Issue("owner", "owner", "user", true)
	if err != nil {
		t.Fatal(err)
	}
	handler := media.NewHandler(zerolog.Nop(), rustfs.NewMediaRepository(client, "media"), accountRepo{account: account})
	mux := http.NewServeMux()
	media.RegisterRoutes(openapi.NewRegistry(mux, false), handler, tokens)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/media/download?key="+key, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}
	if disposition := recorder.Header().Get("Content-Disposition"); disposition != `attachment; filename=front.jpg` {
		t.Fatalf("Content-Disposition = %q", disposition)
	}
}
