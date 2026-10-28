package kyc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/google/uuid"
)

type Media struct {
	Filename    string
	ContentType string
	Data        []byte
}

type VerifyResult struct {
	Success       bool                       `json:"success"`
	Decision      string                     `json:"decision"`
	ReasonCode    string                     `json:"reason_code,omitempty"`
	ReasonMessage string                     `json:"reason_message,omitempty"`
	MatchScore    float64                    `json:"match_score"`
	LivenessScore float64                    `json:"liveness_score"`
	QualityChecks QualityChecks              `json:"quality_checks"`
	Analysis      *accountdomain.KYCEvidence `json:"analysis"`
}

type QualityChecks struct {
	Portrait     bool `json:"cccd_portrait_extracted"`
	SingleFace   bool `json:"single_face_in_video"`
	ImageQuality bool `json:"image_quality_passed"`
}

func (r *VerifyResult) ValidVerified() bool {
	a := r.Analysis
	return r.Success && r.Decision == "VERIFIED" && validScore(r.MatchScore) && validScore(r.LivenessScore) &&
		r.QualityChecks.Portrait && r.QualityChecks.SingleFace && r.QualityChecks.ImageQuality &&
		a != nil && a.RequestID != "" && a.ModelID != "" && a.ModelVersion != "" && len(a.ModelSHA256) == 64 &&
		a.PipelineVersion != "" && a.CalibrationVersion != "" && a.MatchingStatus == "passed" &&
		a.LivenessStatus == "passed" && a.SubjectConsistencyPassed && a.ChallengePassed &&
		a.SampledFrames >= a.MatchedFrames && a.MatchedFrames >= 5 && a.VideoDurationSeconds >= 3 &&
		!math.IsNaN(a.VideoDurationSeconds) && !math.IsInf(a.VideoDurationSeconds, 0)
}

func validScore(s float64) bool { return !math.IsNaN(s) && !math.IsInf(s, 0) && s >= 0 && s <= 1 }

type Client struct {
	baseURL      string
	http         *http.Client
	serviceToken string
}

func NewClient(baseURL string, serviceToken ...string) *Client {
	token := ""
	if len(serviceToken) > 0 {
		token = serviceToken[0]
	}
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		http:         &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		serviceToken: token,
	}
}

func (c *Client) VerifyFace(ctx context.Context, front, video Media, challengeType string) (*VerifyResult, error) {
	if err := c.validateTransport(); err != nil {
		return nil, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writeFile(writer, "cccd_front_image", front); err != nil {
		return nil, err
	}
	if err := writeFile(writer, "live_video", video); err != nil {
		return nil, err
	}
	if err := writer.WriteField("challenge_type", challengeType); err != nil {
		return nil, fmt.Errorf("write challenge type: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close KYC multipart body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/kyc/verify-face", &body)
	if err != nil {
		return nil, fmt.Errorf("create KYC request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	requestID := uuid.NewString()
	req.Header.Set("X-Request-ID", requestID)
	req.Header.Set("X-KYC-Service-Token", c.serviceToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call KYC service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("KYC service returned status %d", resp.StatusCode)
	}
	var result VerifyResult
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode KYC response: %w", err)
	}
	if !validDecision(result.Decision) || (result.Success != (result.Decision == "VERIFIED")) ||
		!validScore(result.MatchScore) || !validScore(result.LivenessScore) ||
		(result.Success && (!result.ValidVerified() || result.Analysis.RequestID != requestID)) {
		return nil, fmt.Errorf("invalid KYC service result")
	}
	return &result, nil
}

func writeFile(writer *multipart.Writer, field string, media Media) error {
	header := make(textproto.MIMEHeader)
	header["Content-Disposition"] = []string{mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": media.Filename})}
	header["Content-Type"] = []string{media.ContentType}
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create %s part: %w", field, err)
	}
	if _, err := part.Write(media.Data); err != nil {
		return fmt.Errorf("write %s part: %w", field, err)
	}
	return nil
}

func validDecision(decision string) bool {
	switch decision {
	case "VERIFIED", "RETRY_ALLOWED", "FAILED_FINAL", "SYSTEM_ERROR":
		return true
	default:
		return false
	}
}

func (c *Client) validateTransport() error {
	u, err := url.Parse(c.baseURL)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("invalid KYC service URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme != "http" {
		return fmt.Errorf("KYC service URL must use HTTPS")
	}
	host := strings.Trim(u.Hostname(), "[]")
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("KYC service URL must use HTTPS outside loopback")
}
