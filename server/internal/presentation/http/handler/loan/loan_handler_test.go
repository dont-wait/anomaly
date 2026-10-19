package loan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	accountqueries "github.com/dont-wait/anomaly/internal/application/account/queries"
	domainauth "github.com/dont-wait/anomaly/internal/domain/auth"
	domain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type downloadTokenStub struct{ accountqueries.TokenService }

func (downloadTokenStub) Parse(string) (*domainauth.Claims, error) {
	return &domainauth.Claims{UserID: "customer-1", Role: "user"}, nil
}

type downloadReaderStub struct{ app *domain.Application }

func (s downloadReaderStub) FindByID(context.Context, string) (*domain.Application, error) {
	return s.app, nil
}

type downloadMediaStub struct{ err error }

func (s downloadMediaStub) Download(context.Context, string) (*rustfs.DownloadResult, error) {
	return nil, s.err
}

func (downloadMediaStub) Upload(context.Context, string, io.Reader, string) error {
	return nil
}

func (downloadMediaStub) Delete(context.Context, string) error { return nil }

func TestDownloadStorageErrors(t *testing.T) {
	for _, tt := range []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"repository sentinel", fmt.Errorf("download: %w", rustfs.ErrObjectNotFound), http.StatusNotFound},
		{"S3 no such key", &types.NoSuchKey{}, http.StatusNotFound},
		{"S3 not found", &smithy.GenericAPIError{Code: "NotFound"}, http.StatusNotFound},
		{"storage outage", errors.New("connection refused"), http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := &domain.Application{
				Id:         "33198444-52f0-4ba0-b161-45ed71e284b9",
				CustomerId: "customer-1",
				Documents:  []domain.ApplicationDocument{{DocumentId: "doc-1", StorageKey: "loans/doc-1"}},
			}
			h := &Handler{Read: downloadReaderStub{app: app}, Media: downloadMediaStub{err: tt.err}}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.SetPathValue("applicationId", app.Id)
			r.SetPathValue("documentId", "doc-1")
			r.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()
			middleware.RequireAuth(downloadTokenStub{})(http.HandlerFunc(h.download)).ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
