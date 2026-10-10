package account_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	kycinfra "github.com/dont-wait/anomaly/internal/infrastructure/kyc"
	handleraccount "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

type verifierStub struct {
	result *kycinfra.VerifyResult
	calls  int
}

func (v *verifierStub) VerifyFace(context.Context, kycinfra.Media, kycinfra.Media, string) (*kycinfra.VerifyResult, error) {
	v.calls++
	return v.result, nil
}

type mediaStoreStub struct{ keys []string }

func (s *mediaStoreStub) Upload(_ context.Context, key string, _ io.Reader, _ string) error {
	s.keys = append(s.keys, key)
	return nil
}
func (*mediaStoreStub) Delete(context.Context, string) error { return nil }

func TestCompleteKYCStoresOnlyVerifiedMedia(t *testing.T) {
	for _, test := range []struct {
		name       string
		decision   string
		success    bool
		wantStored int
		wantVerify bool
	}{
		{name: "retry", decision: "RETRY_ALLOWED"},
		{name: "verified", decision: "VERIFIED", success: true, wantStored: 3, wantVerify: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMemoryAccountRepository()
			account := testAccount(t)
			repo.accounts[account.Id] = account
			tokens := auth.NewTokenService("test-secret", time.Hour)
			verifier := &verifierStub{result: &kycinfra.VerifyResult{Success: test.success, Decision: test.decision}}
			store := &mediaStoreStub{}
			handler := handleraccount.NewHandler(
				zerolog.Nop(), commands.NewRegisterAccountCommandHandler(repo, repo), commands.NewVerifyAccountCommandHandler(repo),
				queries.NewLoginQueryHandler(repo, tokens), queries.NewGetAccountByIDQueryHandler(repo),
				queries.NewGetAccountByEmailQueryHandler(repo), queries.NewGetAllAccountsQueryHandler(repo), tokens, verifier, store,
			)
			mux := http.NewServeMux()
			handleraccount.RegisterRoutes(openapi.NewRegistry(mux, false), handler, tokens)
			token, _, err := tokens.IssueKYC(account.Id)
			if err != nil {
				t.Fatal(err)
			}
			body, contentType := completeBody(t)
			req := httptest.NewRequest(http.MethodPost, "/api/kyc/complete", body)
			req.Header.Set("Content-Type", contentType)
			req.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if len(store.keys) != test.wantStored {
				t.Fatalf("stored objects = %d, want %d", len(store.keys), test.wantStored)
			}
			if account.IsVerified() != test.wantVerify {
				t.Fatalf("verified = %v, want %v", account.IsVerified(), test.wantVerify)
			}
		})
	}
}

func testAccount(t *testing.T) *accountdomain.UserAccount {
	t.Helper()
	now := time.Now().UTC()
	return &accountdomain.UserAccount{Id: "account-id", Username: "alice", CustomerId: "customer-id", Customer: &accountdomain.Customer{
		Id: "customer-id", KYCStatus: accountdomain.KYCStatusNotStarted, CreatedAt: now,
	}}
}

func completeBody(t *testing.T) (io.Reader, string) {
	t.Helper()
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range []string{"idCardFront", "idCardBack"} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+field+`.png"`)
		header.Set("Content-Type", "image/png")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(imageData.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="liveVideo"; filename="live.mp4"`)
	header.Set("Content-Type", "video/mp4")
	video, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := video.Write([]byte{0, 0, 0, 20, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', '2'}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("challengeType", "TURN_HEAD_LEFT_RIGHT_BLINK"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}
