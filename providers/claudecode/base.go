package claudecode

import (
	"context"
	"done-hub/common/cache"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/providers/claude"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const TokenCacheKey = "api_token:claudecode"

// OAuth2 配置常量
const (
	DefaultClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	TokenEndpoint   = "https://console.anthropic.com/v1/oauth/token"
	DefaultScope    = "user:inference user:profile"
)

type ClaudeCodeProviderFactory struct{}

// 创建 ClaudeCodeProvider
func (f ClaudeCodeProviderFactory) Create(channel *model.Channel) base.ProviderInterface {
	provider := &ClaudeCodeProvider{
		ClaudeProvider: claude.ClaudeProvider{
			BaseProvider: base.BaseProvider{
				Config:    getConfig(),
				Channel:   channel,
				Requester: requester.NewHTTPRequester(*channel.Proxy, RequestErrorHandle("")),
			},
		},
	}

	// 解析配置
	parseClaudeCodeConfig(provider)

	// 更新 RequestErrorHandle 使用实际的 token
	if provider.Credentials != nil {
		provider.Requester = requester.NewHTTPRequester(*channel.Proxy, RequestErrorHandle(provider.Credentials.AccessToken))
	}

	return provider
}

// parseClaudeCodeConfig 解析 ClaudeCode 配置
func parseClaudeCodeConfig(provider *ClaudeCodeProvider) {
	channel := provider.Channel

	// 尝试解析完整的 OAuth2 凭证（优先）
	if channel.Key != "" {
		// 尝试解析为 JSON 格式的完整凭证
		creds, err := FromJSON(channel.Key)
		if err == nil {
			// 成功解析为完整凭证
			provider.Credentials = creds
			return
		}

		// 如果解析失败，尝试作为简单的 access_token
		provider.Credentials = &OAuth2Credentials{
			AccessToken: channel.Key,
		}
	}
}

type ClaudeCodeProvider struct {
	claude.ClaudeProvider
	Credentials *OAuth2Credentials
}

func getConfig() base.ProviderConfig {
	return base.ProviderConfig{
		BaseURL:         "https://api.anthropic.com",
		ChatCompletions: "/v1/messages",
		ModelList:       "/v1/models",
	}
}

// RequestErrorHandle 请求错误处理
func RequestErrorHandle(accessToken string) requester.HttpErrorHandler {
	return func(resp *http.Response) *types.OpenAIError {
		claudeError := &claude.ClaudeError{}
		err := json.NewDecoder(resp.Body).Decode(claudeError)
		if err != nil {
			return nil
		}

		openAIError := errorHandle(claudeError)

		// 解析 429 错误的响应头中的冻结时间
		if openAIError != nil && resp.StatusCode == http.StatusTooManyRequests {
			// anthropic-ratelimit-unified-reset: Unix 时间戳（秒）
			resetHeader := resp.Header.Get("anthropic-ratelimit-unified-reset")
			if resetHeader != "" {
				if resetTimestamp, parseErr := strconv.ParseInt(resetHeader, 10, 64); parseErr == nil {
					openAIError.RateLimitResetAt = resetTimestamp
					logger.SysLog(fmt.Sprintf("[ClaudeCode] Rate limit detected, reset at: %d (%s)",
						resetTimestamp, time.Unix(resetTimestamp, 0).Format(time.RFC3339)))
				}
			}
		}

		return openAIError
	}
}

// 错误处理
func errorHandle(claudeError *claude.ClaudeError) *types.OpenAIError {
	if claudeError == nil {
		return nil
	}

	if claudeError.Type == "" {
		return nil
	}

	// 使用 claudecode_error 作为错误类型，以便在 relay/common.go 中进行特殊处理
	// 将原始的 error.type 保存在 Code 字段中
	return &types.OpenAIError{
		Message: claudeError.ErrorInfo.Message,
		Type:    "claudecode_error",
		Code:    claudeError.ErrorInfo.Type, // 保存原始错误类型（如 permission_error）
		Param:   claudeError.Type,           // 保存外层 type（如 error）
	}
}

// GetRequestHeaders 获取请求头
func (p *ClaudeCodeProvider) GetRequestHeaders() map[string]string {
	headers := make(map[string]string)
	p.CommonRequestHeaders(headers)

	token, err := p.GetToken()
	if err != nil {
		if p.Context != nil {
			logger.LogError(p.Context.Request.Context(), "Failed to get ClaudeCode token: "+err.Error())
		} else {
			logger.SysError("Failed to get ClaudeCode token: " + err.Error())
		}
		// 返回空 headers，让后续请求失败
		return headers
	}

	headers["Authorization"] = "Bearer " + token
	headers["anthropic-version"] = "2023-06-01"

	// 从请求中获取 anthropic-version（如果有）
	if p.Context != nil {
		anthropicVersion := p.Context.Request.Header.Get("anthropic-version")
		if anthropicVersion != "" {
			headers["anthropic-version"] = anthropicVersion
		}
	}

	return headers
}

// handleTokenError 处理token获取失败的错误
// Token 刷新失败应该返回 401，触发重试和禁用渠道逻辑
func (p *ClaudeCodeProvider) handleTokenError(err error) *types.OpenAIErrorWithStatusCode {
	errMsg := err.Error()

	// Token 错误统一返回 401 Unauthorized，不设置 LocalError
	// 这样会触发：
	// 1. 重试其他渠道
	// 2. 禁用当前渠道（如果符合禁用条件）
	// 3. 如果所有渠道都失败，返回"上游负载已饱和"
	//
	// 注意：不能使用 StringErrorWrapper，因为它会将 Type 设置为 "one_hub_error"
	// 我们需要 Type 为 "claudecode_token_error" 以便在 FilterOpenAIErr 中正确过滤
	return &types.OpenAIErrorWithStatusCode{
		OpenAIError: types.OpenAIError{
			Message: errMsg,
			Type:    "claudecode_token_error",
			Code:    "claudecode_token_error",
		},
		StatusCode: http.StatusUnauthorized,
		LocalError: false,
	}
}

// GetToken 获取访问令牌，支持自动刷新
// 使用 Redis 缓存，token 刷新后保存到数据库
func (p *ClaudeCodeProvider) GetToken() (string, error) {
	if p.Credentials == nil {
		return "", fmt.Errorf("credentials not configured")
	}

	// 使用固定的缓存 key
	cacheKey := fmt.Sprintf("%s:%d", TokenCacheKey, p.Channel.Id)

	// 1. 尝试从 Redis 缓存获取
	cachedToken, _ := cache.GetCache[string](cacheKey)
	if cachedToken != "" {
		// 缓存命中，直接返回
		return cachedToken, nil
	}

	// 2. 缓存未命中，检查凭证是否过期
	needsUpdate := false
	if p.Credentials.IsExpired() && p.Credentials.RefreshToken != "" {
		// Token 过期且有 refresh_token，尝试刷新
		proxyURL := ""
		if p.Channel.Proxy != nil && *p.Channel.Proxy != "" {
			proxyURL = *p.Channel.Proxy
		}

		// 获取 context
		var ctx context.Context
		if p.Context != nil {
			ctx = p.Context.Request.Context()
		}

		// 刷新 token（最多重试 3 次）
		if err := p.Credentials.Refresh(ctx, proxyURL, 3); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to refresh claudecode token: %s", err.Error()))
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}

		// 标记需要更新数据库
		needsUpdate = true
	}

	// 3. 使用当前的 access_token
	if p.Credentials.AccessToken == "" {
		return "", fmt.Errorf("access token is empty")
	}

	// 4. 如果 token 被刷新，保存新凭证到数据库
	if needsUpdate {
		if err := p.saveCredentialsToDatabase(); err != nil {
			// 获取 context（如果有）
			var ctx context.Context
			if p.Context != nil {
				ctx = p.Context.Request.Context()
			}
			logger.LogError(ctx, fmt.Sprintf("Failed to save refreshed credentials to database: %s", err.Error()))
			// 不返回错误，因为 token 刷新成功了，只是保存失败
		}
	}

	// 5. 将 token 缓存到 Redis
	// 计算缓存时长：如果有过期时间，使用到期前的时间；否则使用默认 55 分钟
	cacheDuration := 55 * time.Minute
	if !p.Credentials.ExpiresAt.IsZero() {
		timeUntilExpiry := time.Until(p.Credentials.ExpiresAt)
		if timeUntilExpiry > 0 && timeUntilExpiry < cacheDuration {
			cacheDuration = timeUntilExpiry
		}
	}

	cache.SetCache(cacheKey, p.Credentials.AccessToken, cacheDuration)

	return p.Credentials.AccessToken, nil
}

// saveCredentialsToDatabase 保存凭证到数据库
func (p *ClaudeCodeProvider) saveCredentialsToDatabase() error {
	// 序列化凭证为 JSON
	credentialsJSON, err := p.Credentials.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize credentials: %w", err)
	}

	// 更新数据库中的 Key 字段
	if err := model.UpdateChannelKey(p.Channel.Id, credentialsJSON); err != nil {
		return fmt.Errorf("failed to update channel key: %w", err)
	}

	logger.SysLog(fmt.Sprintf("[ClaudeCode] Credentials saved to database for channel %d", p.Channel.Id))
	return nil
}

// GetFullRequestURL 获取完整请求URL
func (p *ClaudeCodeProvider) GetFullRequestURL(requestURL string) string {
	baseURL := strings.TrimSuffix(p.GetBaseURL(), "/")
	return fmt.Sprintf("%s%s", baseURL, requestURL)
}
