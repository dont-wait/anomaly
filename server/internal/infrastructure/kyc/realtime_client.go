package kyc

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

type RealtimeError struct{ Code string }

func (e *RealtimeError) Error() string { return fmt.Sprintf("KYC analysis unavailable: %s", e.Code) }

func (c *Client) OpenRealtime(ctx context.Context, front Media, challenge string) (*websocket.Conn, error) {
	if err := c.validateTransport(); err != nil {
		return nil, err
	}
	endpoint := strings.Replace(strings.Replace(c.baseURL, "https://", "wss://", 1), "http://", "ws://", 1) + "/v1/kyc/realtime"
	config, err := websocket.NewConfig(endpoint, c.baseURL)
	if err != nil {
		return nil, err
	}
	config.Header = http.Header{"X-Kyc-Service-Token": []string{c.serviceToken}}
	conn, err := config.DialContext(ctx)
	if err != nil {
		return nil, err
	}
	conn.MaxPayloadBytes = 1 << 20
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	err = websocket.JSON.Send(conn, map[string]string{"front": base64.StdEncoding.EncodeToString(front.Data), "challenge_type": challenge})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	var ready struct {
		Type string `json:"type"`
		Code string `json:"code"`
	}
	if err = websocket.JSON.Receive(conn, &ready); err != nil || ready.Type != "ready" {
		_ = conn.Close()
		if err == nil {
			err = &RealtimeError{Code: ready.Code}
		}
		return nil, err
	}
	return conn, nil
}
