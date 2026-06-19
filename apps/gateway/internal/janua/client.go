package janua

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Client struct {
	baseURL      string
	serviceToken string
	http         *http.Client
}

type DelegationRequest struct {
	Purpose    string `json:"purpose"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type DelegationResponse struct {
	AccessToken  string   `json:"access_token"`
	TokenType    string   `json:"token_type"`
	ExpiresAt    string   `json:"expires_at"`
	Purpose      string   `json:"purpose"`
	ProviderType string   `json:"provider_type"`
	Scopes       []string `json:"scopes"`
}

func NewClient() *Client {
	base := os.Getenv("COUPLER_JANUA_API_URL")
	if base == "" {
		base = "https://auth.madfam.io"
	}
	return &Client{
		baseURL:      base,
		serviceToken: os.Getenv("COUPLER_JANUA_SERVICE_TOKEN"),
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) DelegateToken(ctx context.Context, connectionID, actingUserID string, ttl int) (DelegationResponse, error) {
	var out DelegationResponse
	if c.serviceToken == "" {
		return out, fmt.Errorf("COUPLER_JANUA_SERVICE_TOKEN not configured")
	}
	body, _ := json.Marshal(DelegationRequest{Purpose: "tool_execute", TTLSeconds: ttl})
	url := fmt.Sprintf("%s/api/v1/connections/%s/token", c.baseURL, connectionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", c.serviceToken)
	req.Header.Set("X-Acting-User-Id", actingUserID)

	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return out, fmt.Errorf("janua delegation %d: %s", resp.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

// ResolveConnection picks connection_id or resolves by provider_type for acting user.
func (c *Client) ResolveConnectionID(ctx context.Context, actingUserID, providerType, connectionID, userJWT string) (string, error) {
	if connectionID != "" {
		return connectionID, nil
	}
	if userJWT == "" {
		return "", fmt.Errorf("connection_id required without user JWT for auto-resolve")
	}
	url := fmt.Sprintf("%s/api/v1/connections", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+userJWT)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("janua list connections %d: %s", resp.StatusCode, string(data))
	}
	var parsed struct {
		Connections []struct {
			ID           string `json:"id"`
			ProviderType string `json:"provider_type"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	for _, conn := range parsed.Connections {
		if conn.ProviderType == providerType {
			return conn.ID, nil
		}
	}
	return "", fmt.Errorf("no %s connection for user", providerType)
}
