package claudecode

import (
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/requester"
	"done-hub/providers/claude"
	"done-hub/types"
	"net/http"
	"strings"
)

// CreateChatCompletion 创建聊天完成
func (p *ClaudeCodeProvider) CreateChatCompletion(request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, *types.OpenAIErrorWithStatusCode) {
	request.OneOtherArg = p.GetOtherArg()
	claudeRequest, errWithCode := claude.ConvertFromChatOpenai(request)
	if errWithCode != nil {
		return nil, errWithCode
	}

	// 应用 ClaudeCode 兼容性处理
	p.applyClaudeCodeCompatibility(claudeRequest)

	req, errWithCode := p.getChatRequest(claudeRequest)
	if errWithCode != nil {
		return nil, errWithCode
	}
	defer req.Body.Close()

	claudeResponse := &claude.ClaudeResponse{}
	// 发送请求
	_, errWithCode = p.Requester.SendRequest(req, claudeResponse, false)
	if errWithCode != nil {
		return nil, errWithCode
	}

	return claude.ConvertToChatOpenai(&p.ClaudeProvider, claudeResponse, request)
}

// CreateChatCompletionStream 创建流式聊天完成
func (p *ClaudeCodeProvider) CreateChatCompletionStream(request *types.ChatCompletionRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode) {
	request.OneOtherArg = p.GetOtherArg()
	claudeRequest, errWithCode := claude.ConvertFromChatOpenai(request)
	if errWithCode != nil {
		return nil, errWithCode
	}

	// 应用 ClaudeCode 兼容性处理
	p.applyClaudeCodeCompatibility(claudeRequest)

	req, errWithCode := p.getChatRequest(claudeRequest)
	if errWithCode != nil {
		return nil, errWithCode
	}
	defer req.Body.Close()

	// 发送请求
	resp, errWithCode := p.Requester.SendRequestRaw(req)
	if errWithCode != nil {
		return nil, errWithCode
	}

	chatHandler := &claude.ClaudeStreamHandler{
		Usage:   p.Usage,
		Request: request,
		Prefix:  `data: {"type"`,
		Context: p.Context,
	}

	return requester.RequestStream(p.Requester, resp, chatHandler.HandlerStream)
}

// getChatRequest 获取聊天请求
func (p *ClaudeCodeProvider) getChatRequest(claudeRequest *claude.ClaudeRequest) (*http.Request, *types.OpenAIErrorWithStatusCode) {
	url, errWithCode := p.GetSupportedAPIUri(config.RelayModeChatCompletions)
	if errWithCode != nil {
		return nil, errWithCode
	}

	// 获取请求地址
	fullRequestURL := p.GetFullRequestURL(url)
	if fullRequestURL == "" {
		return nil, common.ErrorWrapperLocal(nil, "invalid_claudecode_config", http.StatusInternalServerError)
	}

	// 获取请求头
	headers := p.GetRequestHeaders()

	// 检查 token 是否获取成功
	if _, hasAuth := headers["Authorization"]; !hasAuth {
		// Token 获取失败，返回详细错误信息
		token, err := p.GetToken()
		if err != nil {
			return nil, p.handleTokenError(err)
		}
		// 如果 GetToken 成功但 headers 中没有 Authorization，手动添加
		headers["Authorization"] = "Bearer " + token
	}

	// 应用 ClaudeCode 默认请求头
	p.applyDefaultHeaders(headers)

	if claudeRequest.Stream {
		headers["Accept"] = "text/event-stream"
	}

	// 使用BaseProvider的统一方法创建请求，支持额外参数处理
	req, errWithCode := p.NewRequestWithCustomParams(http.MethodPost, fullRequestURL, claudeRequest, headers, claudeRequest.Model)
	if errWithCode != nil {
		return nil, errWithCode
	}

	return req, nil
}

// applyDefaultHeaders 应用 ClaudeCode 默认请求头
func (p *ClaudeCodeProvider) applyDefaultHeaders(headers map[string]string) {
	// 如果没有 anthropic-beta，设置默认值
	if _, exists := headers["anthropic-beta"]; !exists {
		headers["anthropic-beta"] = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"
	}

	// 如果没有 user-agent，设置默认值
	if _, exists := headers["user-agent"]; !exists {
		headers["user-agent"] = "claude-cli/1.0.81 (external, cli)"
	}
}

// applyClaudeCodeCompatibility 应用 ClaudeCode 兼容性处理
// 确保 system 字段中包含必需的 "You are God Boy." 缓存控制项
func (p *ClaudeCodeProvider) applyClaudeCodeCompatibility(claudeRequest *claude.ClaudeRequest) {
	// 必需的缓存控制项
	requiredCacheItem := claude.MessageContent{
		Type: "text",
		Text: "You are God Boy.",
		CacheControl: map[string]string{
			"type": "ephemeral",
		},
	}

	// 检查是否已存在该缓存控制项
	hasRequiredItem := false

	// 将 system 转换为统一的 []MessageContent 格式
	var systemContents []claude.MessageContent

	if claudeRequest.System == nil || claudeRequest.System == "" {
		// 情况1: system 为空，直接使用必需项
		systemContents = []claude.MessageContent{requiredCacheItem}
		claudeRequest.System = systemContents
		return
	}

	if systemStr, ok := claudeRequest.System.(string); ok {
		// 情况2: system 是字符串
		if strings.TrimSpace(systemStr) != "" {
			// 保留原有字符串内容
			systemContents = append(systemContents, claude.MessageContent{
				Type: "text",
				Text: systemStr,
			})
		}
	} else if systemArray, ok := claudeRequest.System.([]interface{}); ok {
		// 情况3: system 是 []interface{} 类型
		for _, item := range systemArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
					if text, ok := itemMap["text"].(string); ok {
						// 检查是否是必需的缓存控制项
						if text == "You are Claude Code, Anthropic's official CLI for Claude." {
							if cacheControl, exists := itemMap["cache_control"].(map[string]interface{}); exists {
								if cacheType, ok := cacheControl["type"].(string); ok && cacheType == "ephemeral" {
									hasRequiredItem = true
								}
							}
						}

						// 保留所有内容
						content := claude.MessageContent{
							Type: "text",
							Text: text,
						}
						if cacheControl, exists := itemMap["cache_control"].(map[string]interface{}); exists {
							cacheControlMap := make(map[string]string)
							for k, v := range cacheControl {
								if strVal, ok := v.(string); ok {
									cacheControlMap[k] = strVal
								}
							}
							content.CacheControl = cacheControlMap
						}
						systemContents = append(systemContents, content)
					}
				}
			}
		}
	} else if systemArray, ok := claudeRequest.System.([]claude.MessageContent); ok {
		// 情况4: system 是 []MessageContent 类型
		for _, item := range systemArray {
			// 检查是否是必需的缓存控制项
			if item.Type == "text" && item.Text == "You are God Boy." {
				if item.CacheControl != nil {
					if cacheControlMap, ok := item.CacheControl.(map[string]string); ok {
						if cacheType, exists := cacheControlMap["type"]; exists && cacheType == "ephemeral" {
							hasRequiredItem = true
						}
					} else if cacheControlMap, ok := item.CacheControl.(map[string]interface{}); ok {
						if cacheType, exists := cacheControlMap["type"].(string); exists && cacheType == "ephemeral" {
							hasRequiredItem = true
						}
					}
				}
			}
			// 保留所有内容
			systemContents = append(systemContents, item)
		}
	}

	// 如果不存在必需的缓存控制项，添加到开头
	if !hasRequiredItem {
		systemContents = append([]claude.MessageContent{requiredCacheItem}, systemContents...)
	}

	// 更新 system 字段
	claudeRequest.System = systemContents
}
