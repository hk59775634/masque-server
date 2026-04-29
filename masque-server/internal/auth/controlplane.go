package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL       string
	HTTPClient    *http.Client
	HMACSecret    string
	NowUnix       func() int64
	SignHeaderTS  string
	SignHeaderMAC string
}

type authorizeRequest struct {
	DeviceToken string `json:"device_token"`
	Fingerprint string `json:"fingerprint"`
}

type AuthorizeResponse struct {
	Allowed  bool           `json:"allowed"`
	DeviceID int            `json:"device_id"`
	UserID   int            `json:"user_id"`
	ACL      map[string]any `json:"acl"`
	Routes   []string       `json:"routes"`
	DNS      []string       `json:"dns"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:       baseURL,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		NowUnix:       func() int64 { return time.Now().Unix() },
		SignHeaderTS:  "X-Masque-Authz-Timestamp",
		SignHeaderMAC: "X-Masque-Authz-Signature",
	}
}

func (c *Client) Authorize(ctx context.Context, deviceToken string, fingerprint string) (*AuthorizeResponse, error) {
	payload, _ := json.Marshal(authorizeRequest{
		DeviceToken: deviceToken,
		Fingerprint: fingerprint,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/server/authorize", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.signAuthorizeRequest(req, payload)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out AuthorizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest || !out.Allowed {
		return nil, fmt.Errorf("authorization denied")
	}

	return &out, nil
}

func (c *Client) signAuthorizeRequest(req *http.Request, body []byte) {
	secret := strings.TrimSpace(c.HMACSecret)
	if secret == "" {
		return
	}
	nowFn := c.NowUnix
	if nowFn == nil {
		nowFn = func() int64 { return time.Now().Unix() }
	}
	ts := fmt.Sprintf("%d", nowFn())
	payloadHash := sha256.Sum256(body)
	macPayload := strings.Join([]string{
		req.Method,
		req.URL.Path,
		ts,
		hex.EncodeToString(payloadHash[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(macPayload))
	req.Header.Set(c.SignHeaderTS, ts)
	req.Header.Set(c.SignHeaderMAC, hex.EncodeToString(mac.Sum(nil)))
}
