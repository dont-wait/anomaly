package kyc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"golang.org/x/net/websocket"
)

func TestRealtimeClientAuthenticatesAndStreamsOrderedMedia(t *testing.T) {
	server := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		if ws.Request().Header.Get("X-KYC-Service-Token") != "internal-secret" {
			t.Error("service authentication missing")
			return
		}
		var init map[string]string
		if err := websocket.JSON.Receive(ws, &init); err != nil {
			t.Error(err)
			return
		}
		front, err := base64.StdEncoding.DecodeString(init["front"])
		if err != nil || string(front) != "front" {
			t.Error("reference image differs")
			return
		}
		websocket.JSON.Send(ws, map[string]string{"type": "ready"})
		var start map[string]string
		if err := websocket.JSON.Receive(ws, &start); err != nil || start["type"] != "start" {
			t.Error("missing start")
			return
		}
		var chunk []byte
		if err := websocket.Message.Receive(ws, &chunk); err != nil || string(chunk) != "\x01\x00\x00\x00\x00media" {
			t.Error("chunk changed")
			return
		}
		var end map[string]any
		if err := websocket.JSON.Receive(ws, &end); err != nil || end["type"] != "end" {
			t.Error("missing end")
			return
		}
		websocket.JSON.Send(ws, map[string]string{"type": "stream_finished"})
	}))
	defer server.Close()
	conn, err := NewClient(server.URL, "internal-secret").OpenRealtime(context.Background(), Media{Data: []byte("front")}, "TURN_HEAD_LEFT_RIGHT_BLINK")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	websocket.JSON.Send(conn, map[string]string{"type": "start"})
	websocket.Message.Send(conn, []byte("\x01\x00\x00\x00\x00media"))
	websocket.JSON.Send(conn, map[string]any{"type": "end", "lastChunkSeq": 0})
	var event map[string]string
	if err := websocket.JSON.Receive(conn, &event); err != nil || event["type"] != "stream_finished" {
		t.Fatalf("stream did not complete: %v", err)
	}
}

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
			"liveness_score": 0.98, "quality_checks": map[string]bool{"image_quality_passed": true, "cccd_portrait_extracted": true, "single_face_in_video": true},
			"analysis": accountdomain.KYCEvidence{RequestID: r.Header.Get("X-Request-ID"), ModelID: "test-model", ModelVersion: "v1", ModelSHA256: strings.Repeat("a", 64), PipelineVersion: "v1", CalibrationVersion: "v1", MatchingStatus: "passed", LivenessStatus: "passed", SubjectConsistencyPassed: true, ChallengePassed: true, SampledFrames: 100, MatchedFrames: 8, VideoDurationSeconds: 12},
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

func TestVerifyFaceRejectsUnsubstantiatedVerifiedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"decision":"VERIFIED","match_score":0.99,"liveness_score":0.99}`))
	}))
	defer server.Close()
	if _, err := NewClient(server.URL).VerifyFace(context.Background(), Media{}, Media{}, "BLINK"); err == nil {
		t.Fatal("verified without actual evidence accepted")
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
