package kyc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

type Media struct {
	Filename    string
	ContentType string
	Data        []byte
}

type VerifyResult struct {
	Success       bool    `json:"success"`
	Decision      string  `json:"decision"`
	ReasonCode    string  `json:"reason_code,omitempty"`
	ReasonMessage string  `json:"reason_message,omitempty"`
	MatchScore    float64 `json:"match_score"`
	LivenessScore float64 `json:"liveness_score"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *Client) VerifyFace(ctx context.Context, front, video Media, challengeType string) (*VerifyResult, error) {
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
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call KYC service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("KYC service returned status %d", resp.StatusCode)
	}
	var result VerifyResult
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode KYC response: %w", err)
	}
	if !validDecision(result.Decision) || (result.Success != (result.Decision == "VERIFIED")) {
		return nil, fmt.Errorf("invalid KYC service result")
	}
	return &result, nil
}

func writeFile(writer *multipart.Writer, field string, media Media) error {
	header := make(textproto.MIMEHeader)
	header["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, strings.ReplaceAll(media.Filename, `"`, ""))}
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
