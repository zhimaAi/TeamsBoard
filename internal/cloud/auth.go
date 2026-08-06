package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

	"goteams-client/internal/protocol"
)

// UserInfo cloud user information
type UserInfo struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
	Role        string `json:"role"`
}

// LoginResponse login response
type LoginResponse struct {
	Token        string   `json:"token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresAt    int64    `json:"expires_at"`
	User         UserInfo `json:"user"`
	AdminID      string   `json:"admin_id"`
	TenantID     string   `json:"tenant_id"`
}

// DeviceCredential Credentials returned by device registration
type DeviceCredential struct {
	Credential string `json:"credential"`
	DeviceUUID string `json:"device_uuid"`
}

// Login calls the cloud login interface
func (c *Client) Login(ctx context.Context, username, password string) (*LoginResponse, error) {
	body := map[string]string{
		"username": username,
		"password": password,
	}

	// The current cloud login data is a flat structure; the User/RefreshToken field is retained to be compatible with subsequent versions.
	var payload struct {
		Token        string   `json:"token"`
		RefreshToken string   `json:"refresh_token"`
		ExpiresAt    int64    `json:"expires_at"`
		AdminID      string   `json:"admin_id"`
		TenantID     string   `json:"tenant_id"`
		ID           string   `json:"id"`
		Username     string   `json:"username"`
		Name         string   `json:"name"`
		Avatar       string   `json:"avatar"`
		RoleID       string   `json:"role_id"`
		User         UserInfo `json:"user"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/auth/login", body, &payload); err != nil {
		return nil, err
	}
	if payload.User.ID == "" {
		payload.User = UserInfo{
			ID:          payload.ID,
			Username:    payload.Username,
			DisplayName: payload.Name,
			Avatar:      payload.Avatar,
			Role:        payload.RoleID,
		}
	}
	if payload.User.Username == "" {
		payload.User.Username = username
	}
	if payload.Token == "" || payload.AdminID == "" || payload.User.ID == "" {
		return nil, fmt.Errorf("云端登录响应缺少 token、admin_id 或用户 ID")
	}
	return &LoginResponse{
		Token:        payload.Token,
		RefreshToken: payload.RefreshToken,
		ExpiresAt:    payload.ExpiresAt,
		User:         payload.User,
		AdminID:      payload.AdminID,
		TenantID:     payload.TenantID,
	}, nil
}

// RefreshToken refresh JWT
func (c *Client) RefreshToken(ctx context.Context, token string) (*LoginResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/user/refreshToken", nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("刷新令牌失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(resp)
	}

	var result LoginResponse
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if err := decodeAPIResponse(resp.StatusCode, respBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetMe gets the current user information
func (c *Client) GetMe(ctx context.Context) (*UserInfo, error) {
	var result UserInfo
	if err := c.do(ctx, http.MethodGet, "/api/client/me", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RegisterDevice registers the device (using the boot key)
func (c *Client) RegisterDevice(ctx context.Context, deviceUUID, bootstrapKey string) (*DeviceCredential, error) {
	deviceName, _ := os.Hostname()
	body := map[string]interface{}{
		"device_uuid":      deviceUUID,
		"device_name":      deviceName,
		"os":               runtime.GOOS,
		"arch":             runtime.GOARCH,
		"client_version":   c.cloudCfg.ClientVersion,
		"protocol_version": protocol.ProtocolVersion,
	}

	url := c.baseURL + "/api/client/devices/register"
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.injectBootstrapHeaders(req, bootstrapKey)
	req.Header.Set("X-GoTeams-Device-ID", deviceUUID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("设备注册请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(resp)
	}

	var result DeviceCredential
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if err := decodeAPIResponse(resp.StatusCode, respBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetCompatibility checks client version compatibility
func (c *Client) GetCompatibility(ctx context.Context) (bool, error) {
	var result struct {
		Compatible      bool   `json:"compatible"`
		LatestVersion   string `json:"latest_version"`
		UpgradeRequired bool   `json:"upgrade_required"`
		Message         string `json:"message"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/client/compatibility", nil, &result); err != nil {
		return false, err
	}
	return result.Compatible, nil
}
