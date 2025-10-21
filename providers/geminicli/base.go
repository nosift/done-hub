package geminicli

import (
	"bytes"
	"done-hub/common/cache"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/providers/gemini"
	"done-hub/providers/openai"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const TokenCacheKey = "api_token:geminicli"

// OAuth2 配置常量
const (
	DefaultClientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	DefaultClientSecret = "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"
	TokenEndpoint       = "https://oauth2.googleapis.com/token"
	DefaultScope        = "https://www.googleapis.com/auth/cloud-platform"
)

type GeminiCliProviderFactory struct{}

// 创建 GeminiCliProvider
func (f GeminiCliProviderFactory) Create(channel *model.Channel) base.ProviderInterface {
	provider := &GeminiCliProvider{
		GeminiProvider: gemini.GeminiProvider{
			OpenAIProvider: openai.OpenAIProvider{
				BaseProvider: base.BaseProvider{
					Config:    getConfig("https://cloudcode-pa.googleapis.com"),
					Channel:   channel,
					Requester: requester.NewHTTPRequester(*channel.Proxy, RequestErrorHandle("")),
				},
				SupportStreamOptions: true,
			},
		},
	}

	// 解析配置
	parseGeminiCliConfig(provider)

	// 更新 RequestErrorHandle 使用实际的 token
	if provider.Credentials != nil {
		provider.Requester = requester.NewHTTPRequester(*channel.Proxy, RequestErrorHandle(provider.Credentials.AccessToken))
	}

	return provider
}

// parseGeminiCliConfig 解析 GeminiCli 配置
func parseGeminiCliConfig(provider *GeminiCliProvider) {
	channel := provider.Channel

	// 默认配置
	endpoint := "https://cloudcode-pa.googleapis.com"

	// 尝试从 Plugin 中获取配置
	if channel.Plugin != nil {
		plugin := channel.Plugin.Data()
		if geminicliConfig, ok := plugin["geminicli"]; ok {
			if epVal, ok := geminicliConfig["endpoint"]; ok {
				if ep, ok := epVal.(string); ok && ep != "" {
					endpoint = ep
				}
			}
		}
	}

	provider.Endpoint = endpoint
	provider.Config = getConfig(endpoint)

	// 尝试解析完整的 OAuth2 凭证（优先）
	if channel.Key != "" {
		// 尝试解析为 JSON 格式的完整凭证
		creds, err := FromJSON(channel.Key)
		if err == nil && creds.ProjectID != "" {
			// 成功解析为完整凭证
			provider.Credentials = creds
			provider.ProjectID = creds.ProjectID
			return
		}

		// 尝试解析为简单格式: project_id|access_token
		parts := strings.SplitN(channel.Key, "|", 2)
		if len(parts) == 2 {
			provider.ProjectID = parts[0]
			provider.Credentials = &OAuth2Credentials{
				AccessToken: parts[1],
				ProjectID:   parts[0],
			}
			return
		}
	}

	// 从 Plugin 中获取配置（兼容旧版本）
	if channel.Plugin != nil {
		plugin := channel.Plugin.Data()
		if geminicliConfig, ok := plugin["geminicli"]; ok {
			projectID := ""
			accessToken := ""

			if pidVal, ok := geminicliConfig["project_id"]; ok {
				if pid, ok := pidVal.(string); ok && pid != "" {
					projectID = pid
				}
			}
			if tokenVal, ok := geminicliConfig["access_token"]; ok {
				if token, ok := tokenVal.(string); ok && token != "" {
					accessToken = token
				}
			}

			if projectID != "" && accessToken != "" {
				provider.ProjectID = projectID
				provider.Credentials = &OAuth2Credentials{
					AccessToken: accessToken,
					ProjectID:   projectID,
				}
			}
		}
	}
}

type GeminiCliProvider struct {
	gemini.GeminiProvider
	Endpoint    string
	ProjectID   string
	Credentials *OAuth2Credentials // OAuth2 凭证（包含 refresh_token）
}

func getConfig(endpoint string) base.ProviderConfig {
	return base.ProviderConfig{
		BaseURL:           endpoint,
		ChatCompletions:   "/v1internal/chat/completions",
		ModelList:         "/models",
		ImagesGenerations: "1",
	}
}

// 请求错误处理
func RequestErrorHandle(token string) requester.HttpErrorHandler {
	return func(resp *http.Response) *types.OpenAIError {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil
		}
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		geminiError := &gemini.GeminiErrorResponse{}
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		if err := json.NewDecoder(resp.Body).Decode(geminiError); err == nil {
			return errorHandle(geminiError, token)
		}

		geminiErrors := &gemini.GeminiErrors{}
		if err := json.Unmarshal(bodyBytes, geminiErrors); err == nil {
			return errorHandle(geminiErrors.Error(), token)
		}

		return nil
	}
}

// 错误处理
func errorHandle(geminiError *gemini.GeminiErrorResponse, token string) *types.OpenAIError {
	if geminiError.ErrorInfo == nil || geminiError.ErrorInfo.Message == "" {
		return nil
	}

	cleaningError(geminiError.ErrorInfo, token)

	return &types.OpenAIError{
		Message: geminiError.ErrorInfo.Message,
		Type:    "geminicli_error",
		Param:   geminiError.ErrorInfo.Status,
		Code:    geminiError.ErrorInfo.Code,
	}
}

func cleaningError(errorInfo *gemini.GeminiError, token string) {
	if token == "" {
		return
	}
	message := strings.Replace(errorInfo.Message, token, "xxxxx", 1)
	errorInfo.Message = message
}

func (p *GeminiCliProvider) GetFullRequestURL(requestURL string, modelName string) string {
	baseURL := strings.TrimSuffix(p.GetBaseURL(), "/")

	// GeminiCli使用内部API格式
	// 例如: https://cloudcode-pa.googleapis.com/v1internal:generateContent
	return fmt.Sprintf("%s/v1internal:%s", baseURL, requestURL)
}

// GetToken 获取访问令牌，支持自动刷新
// 完全参考 VertexAI 的实现：使用 Redis 缓存，不保存到数据库
func (p *GeminiCliProvider) GetToken() (string, error) {
	if p.Credentials == nil {
		return "", fmt.Errorf("credentials not configured")
	}

	// 使用 project ID 作为缓存 key（与 VertexAI 保持一致）
	cacheKey := fmt.Sprintf("%s:%s", TokenCacheKey, p.ProjectID)

	// 1. 尝试从 Redis 缓存获取
	cachedToken, err := cache.GetCache[string](cacheKey)
	if err != nil {
		logger.SysError("Failed to get geminicli token from cache: " + err.Error())
	}

	if cachedToken != "" {
		// 缓存命中，直接返回
		return cachedToken, nil
	}

	// 2. 缓存未命中，检查凭证是否过期
	if p.Credentials.IsExpired() && p.Credentials.RefreshToken != "" {
		// Token 过期且有 refresh_token，尝试刷新
		proxyURL := ""
		if p.Channel.Proxy != nil && *p.Channel.Proxy != "" {
			proxyURL = *p.Channel.Proxy
		}

		// 刷新 token（最多重试 3 次）
		if err := p.Credentials.Refresh(proxyURL, 3); err != nil {
			logger.SysError(fmt.Sprintf("Failed to refresh geminicli token: %s", err.Error()))
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
	}

	// 3. 使用当前的 access_token
	if p.Credentials.AccessToken == "" {
		return "", fmt.Errorf("access token is empty")
	}

	// 4. 计算缓存时长（与 VertexAI 保持一致）
	// 如果有过期时间，缓存到过期前 5 分钟
	// 否则默认缓存 30 分钟
	duration := 30 * time.Minute
	if !p.Credentials.ExpiresAt.IsZero() {
		timeUntilExpiry := time.Until(p.Credentials.ExpiresAt)
		if timeUntilExpiry > 5*time.Minute {
			duration = timeUntilExpiry - 5*time.Minute
		} else if timeUntilExpiry > 0 {
			duration = timeUntilExpiry
		}
	}

	// 5. 缓存 token 到 Redis
	cache.SetCache(cacheKey, p.Credentials.AccessToken, duration)

	return p.Credentials.AccessToken, nil
}

// getRequestHeadersInternal 内部方法，返回请求头和错误信息
// 参考 VertexAI 的实现
func (p *GeminiCliProvider) getRequestHeadersInternal() (headers map[string]string, err error) {
	headers = make(map[string]string)
	p.CommonRequestHeaders(headers)

	// 获取 token（会自动刷新如果过期）
	token, err := p.GetToken()
	if err != nil {
		logger.SysError("Failed to get geminicli token: " + err.Error())
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
	headers["Content-Type"] = "application/json"

	return headers, nil
}

// 获取请求头
func (p *GeminiCliProvider) GetRequestHeaders() (headers map[string]string) {
	headers, _ = p.getRequestHeadersInternal()
	return headers
}
