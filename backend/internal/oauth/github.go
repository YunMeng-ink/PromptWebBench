// Package oauth 封装 GitHub OAuth 授权与用户信息读取。
// 各端点地址可替换，测试用本地 HTTP 服务驱动真实客户端代码。
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAuthorizeURL = "https://github.com/login/oauth/authorize"
	defaultTokenURL     = "https://github.com/login/oauth/access_token"
	defaultUserURL      = "https://api.github.com/user"
	defaultEmailsURL    = "https://api.github.com/user/emails"
	maxResponseBytes    = 1 << 20
)

// ErrNotConfigured 表示未配置客户端凭据。
var ErrNotConfigured = errors.New("GitHub OAuth 未配置")

// ErrExchangeFailed 表示授权码换取访问令牌失败。
var ErrExchangeFailed = errors.New("GitHub 授权码校验失败")

// GitHubUser 是登录所需的 GitHub 用户信息。
type GitHubUser struct {
	ID            int64
	Login         string
	Name          string
	Email         string
	EmailVerified bool
}

// Client 是 GitHub OAuth 客户端。四个端点可替换，测试用本地 HTTP 服务驱动真实客户端代码。
type Client struct {
	ClientID          string
	ClientSecret      string
	AuthorizeEndpoint string
	TokenEndpoint     string
	UserEndpoint      string
	EmailsEndpoint    string
	HTTPClient        *http.Client
}

func NewGitHubClient(clientID, clientSecret string) *Client {
	return &Client{
		ClientID:          clientID,
		ClientSecret:      clientSecret,
		AuthorizeEndpoint: defaultAuthorizeURL,
		TokenEndpoint:     defaultTokenURL,
		UserEndpoint:      defaultUserURL,
		EmailsEndpoint:    defaultEmailsURL,
		HTTPClient:        &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

// AuthorizeURL 拼接授权跳转地址。
func (c *Client) AuthorizeURL(state, redirectURI string) string {
	query := url.Values{}
	query.Set("client_id", c.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", "read:user user:email")
	query.Set("state", state)
	return c.AuthorizeEndpoint + "?" + query.Encode()
}

// Exchange 用授权码换取访问令牌。
func (c *Client) Exchange(ctx context.Context, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("构造令牌请求: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 GitHub 令牌端点: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimited(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("GitHub 令牌端点返回 %d", resp.StatusCode)
	}

	var payload struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析令牌响应: %w", err)
	}
	if payload.Error != "" || payload.AccessToken == "" {
		return "", fmt.Errorf("%w: %s %s", ErrExchangeFailed, payload.Error, payload.ErrorDescription)
	}
	return payload.AccessToken, nil
}

// FetchUser 读取 GitHub 用户信息；接口未返回邮箱时改用邮箱列表端点补全。
func (c *Client) FetchUser(ctx context.Context, accessToken string) (GitHubUser, error) {
	var payload struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.getJSON(ctx, c.UserEndpoint, accessToken, &payload); err != nil {
		return GitHubUser{}, err
	}
	if payload.ID == 0 {
		return GitHubUser{}, fmt.Errorf("GitHub 用户响应缺少 id")
	}

	user := GitHubUser{ID: payload.ID, Login: payload.Login, Name: payload.Name, Email: payload.Email}
	if user.Email != "" {
		user.EmailVerified = true
		return user, nil
	}

	email, verified, err := c.fetchPrimaryEmail(ctx, accessToken)
	if err != nil {
		return GitHubUser{}, err
	}
	user.Email = email
	user.EmailVerified = verified
	return user, nil
}

func (c *Client) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, bool, error) {
	var payload []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := c.getJSON(ctx, c.EmailsEndpoint, accessToken, &payload); err != nil {
		return "", false, err
	}

	for _, item := range payload {
		if item.Primary {
			return item.Email, item.Verified, nil
		}
	}
	if len(payload) > 0 {
		return payload[0].Email, payload[0].Verified, nil
	}
	return "", false, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint, accessToken string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("构造请求 %s: %w", endpoint, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := readLimited(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s 返回 %s", endpoint, strconv.Itoa(resp.StatusCode))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("解析 %s 响应: %w", endpoint, err)
	}
	return nil
}

func readLimited(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("读取响应体: %w", err)
	}
	return raw, nil
}
