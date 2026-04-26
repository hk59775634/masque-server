package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
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
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
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
