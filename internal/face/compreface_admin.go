package face

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	defaultCompreFaceApplication = "FaceSign"
	defaultCompreFaceService     = "FaceSign Recognition"
	comprefaceBasicAuthorization = "Basic Q29tbW9uQ2xpZW50SWQ6cGFzc3dvcmQ="
)

type CompreFaceProvisionOptions struct {
	BaseURL  string
	Email    string
	Password string
}

type CompreFaceProvisionResult struct {
	APIKey string
}

type comprefaceAdminClient struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

type comprefaceApplication struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type comprefaceModel struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"apiKey"`
	Type   string `json:"type"`
}

func ProvisionCompreFace(ctx context.Context, options CompreFaceProvisionOptions) (CompreFaceProvisionResult, error) {
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	options.Email = strings.TrimSpace(options.Email)
	if options.BaseURL == "" || options.Email == "" || options.Password == "" {
		return CompreFaceProvisionResult{}, fmt.Errorf("CompreFace 地址、管理员邮箱和密码不能为空")
	}
	parsed, err := url.Parse(options.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return CompreFaceProvisionResult{}, fmt.Errorf("CompreFace 地址必须是有效的 HTTP/HTTPS 地址")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return CompreFaceProvisionResult{}, fmt.Errorf("create cookie jar: %w", err)
	}
	client := &comprefaceAdminClient{
		baseURL: options.BaseURL,
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 20 * time.Second,
		},
	}

	registrationErr := client.register(ctx, options.Email, options.Password)
	if err := client.login(ctx, options.Email, options.Password); err != nil {
		if registrationErr != nil {
			return CompreFaceProvisionResult{}, fmt.Errorf("登录 CompreFace 管理端失败；注册结果：%v；登录结果：%w", registrationErr, err)
		}
		return CompreFaceProvisionResult{}, fmt.Errorf("登录 CompreFace 管理端失败: %w", err)
	}

	application, err := client.ensureApplication(ctx)
	if err != nil {
		return CompreFaceProvisionResult{}, err
	}
	model, err := client.ensureRecognitionService(ctx, application.ID)
	if err != nil {
		return CompreFaceProvisionResult{}, err
	}
	if strings.TrimSpace(model.APIKey) == "" {
		return CompreFaceProvisionResult{}, fmt.Errorf("CompreFace 返回了空的识别服务 API Key")
	}
	if err := client.verifyRecognitionService(ctx, model.APIKey); err != nil {
		return CompreFaceProvisionResult{}, fmt.Errorf("验证自动创建的 CompreFace 识别服务失败: %w", err)
	}
	return CompreFaceProvisionResult{APIKey: model.APIKey}, nil
}

func (c *comprefaceAdminClient) register(ctx context.Context, email, password string) error {
	body := map[string]any{
		"email":             email,
		"firstName":         "FaceSign",
		"lastName":          "Administrator",
		"password":          password,
		"isAllowStatistics": false,
	}
	return c.doJSON(ctx, http.MethodPost, "/admin/user/register", body, nil)
}

func (c *comprefaceAdminClient) login(ctx context.Context, email, password string) error {
	form := url.Values{}
	form.Set("username", email)
	form.Set("password", password)
	form.Set("grant_type", "password")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/admin/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", comprefaceBasicAuthorization)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.send(req, &response); err != nil {
		return err
	}
	c.token = response.AccessToken
	return nil
}

func (c *comprefaceAdminClient) ensureApplication(ctx context.Context) (comprefaceApplication, error) {
	var applications []comprefaceApplication
	if err := c.doJSON(ctx, http.MethodGet, "/admin/apps", nil, &applications); err != nil {
		return comprefaceApplication{}, fmt.Errorf("读取 CompreFace 应用失败: %w", err)
	}
	for _, application := range applications {
		if application.Name == defaultCompreFaceApplication {
			return application, nil
		}
	}

	var application comprefaceApplication
	if err := c.doJSON(ctx, http.MethodPost, "/admin/app", map[string]string{"name": defaultCompreFaceApplication}, &application); err != nil {
		return comprefaceApplication{}, fmt.Errorf("创建 CompreFace 应用失败: %w", err)
	}
	if application.ID == "" {
		return comprefaceApplication{}, fmt.Errorf("CompreFace 创建应用后未返回编号")
	}
	return application, nil
}

func (c *comprefaceAdminClient) ensureRecognitionService(ctx context.Context, applicationID string) (comprefaceModel, error) {
	path := "/admin/app/" + url.PathEscape(applicationID) + "/models"
	var models []comprefaceModel
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &models); err != nil {
		return comprefaceModel{}, fmt.Errorf("读取 CompreFace 服务失败: %w", err)
	}
	for _, model := range models {
		if model.Name == defaultCompreFaceService && strings.EqualFold(model.Type, "RECOGNITION") {
			return model, nil
		}
	}

	path = "/admin/app/" + url.PathEscape(applicationID) + "/model"
	var model comprefaceModel
	request := map[string]string{"name": defaultCompreFaceService, "type": "RECOGNITION"}
	if err := c.doJSON(ctx, http.MethodPost, path, request, &model); err != nil {
		return comprefaceModel{}, fmt.Errorf("创建 CompreFace 人脸识别服务失败: %w", err)
	}
	return model, nil
}

func (c *comprefaceAdminClient) verifyRecognitionService(ctx context.Context, apiKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/recognition/subjects/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", apiKey)
	return c.send(req, nil)
}

func (c *comprefaceAdminClient) doJSON(ctx context.Context, method, path string, body, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.send(req, target)
}

func (c *comprefaceAdminClient) send(req *http.Request, target any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("HTTP %s: %s", resp.Status, bytes.TrimSpace(message))
	}
	if target == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil {
		return fmt.Errorf("解析 CompreFace 响应失败: %w", err)
	}
	return nil
}
