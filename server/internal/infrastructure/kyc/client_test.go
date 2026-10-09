package kyc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyFaceUsesPythonServiceContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/kyc/verify-face" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("challenge_type") != "BLINK" {
			t.Fatalf("challenge_type = %q", r.FormValue("challenge_type"))
		}
		for _, field := range []string{"cccd_front_image", "live_video"} {
			if len(r.MultipartForm.File[field]) != 1 {
				t.Fatalf("missing multipart field %s", field)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "decision": "VERIFIED", "match_score": 0.99,
			"liveness_score": 0.98, "quality_checks": map[string]bool{"image_quality_passed": true},
		})
	}))
	defer server.Close()

	result, err := NewClient(server.URL).VerifyFace(context.Background(),
		Media{Filename: "front.png", ContentType: "image/png", Data: []byte("front")},
		Media{Filename: "live.mp4", ContentType: "video/mp4", Data: []byte("video")},
		"BLINK",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.Decision != "VERIFIED" {
		t.Fatalf("result = %#v", result)
	}
}

func TestVerifyFaceRejectsInconsistentVerifiedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"decision":"VERIFIED"}`))
	}))
	defer server.Close()
	_, err := NewClient(server.URL).VerifyFace(context.Background(), Media{}, Media{}, "BLINK")
	if err == nil {
		t.Fatal("expected inconsistent response to be rejected")
	}
}
