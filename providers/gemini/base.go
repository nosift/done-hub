package gemini

import (
	"bytes"
	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/providers/openai"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GeminiProviderFactory struct{}

// 创建 GeminiProvider
func (f GeminiProviderFactory) Create(channel *model.Channel) base.ProviderInterface {
	useOpenaiAPI := false
	useCodeExecution := false

	if channel.Plugin != nil {
		plugin := channel.Plugin.Data()
		if pWeb, ok := plugin["code_execution"]; ok {
			if enable, ok := pWeb["enable"].(bool); ok && enable {
				useCodeExecution = true
			}
		}

		if pWeb, ok := plugin["use_openai_api"]; ok {
			if enable, ok := pWeb["enable"].(bool); ok && enable {
				useOpenaiAPI = true
			}
		}
	}

	version := "v1beta"
	if channel.Other != "" {
		version = channel.Other
	}

	return &GeminiProvider{
		OpenAIProvider: openai.OpenAIProvider{
			BaseProvider: base.BaseProvider{
				Config:    getConfig(version),
				Channel:   channel,
				Requester: requester.NewHTTPRequester(*channel.Proxy, RequestErrorHandle(channel.Key)),
			},
			SupportStreamOptions: true,
		},
		UseOpenaiAPI:     useOpenaiAPI,
		UseCodeExecution: useCodeExecution,
	}
}

type GeminiProvider struct {
	openai.OpenAIProvider
	UseOpenaiAPI     bool
	UseCodeExecution bool
}

func getConfig(version string) base.ProviderConfig {
	return base.ProviderConfig{
		BaseURL:           "https://generativelanguage.googleapis.com",
		ChatCompletions:   fmt.Sprintf("/%s/chat/completions", version),
		ModelList:         "/models",
		ImagesGenerations: "1",
	}
}

// 请求错误处理
func RequestErrorHandle(key string) requester.HttpErrorHandler {
	return func(resp *http.Response) *types.OpenAIError {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil
		}
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		geminiError := &GeminiErrorResponse{}
		if err := json.NewDecoder(resp.Body).Decode(geminiError); err == nil {
			return errorHandle(geminiError, key)
		} else {
			geminiErrors := &GeminiErrors{}
			if err := json.Unmarshal(bodyBytes, geminiErrors); err == nil {
				return errorHandle(geminiErrors.Error(), key)
			}
		}

		return nil
	}
}

// 错误处理
func errorHandle(geminiError *GeminiErrorResponse, key string) *types.OpenAIError {
	if geminiError.ErrorInfo == nil || geminiError.ErrorInfo.Message == "" {
		return nil
	}

	cleaningError(geminiError.ErrorInfo, key)

	return &types.OpenAIError{
		Message: geminiError.ErrorInfo.Message,
		Type:    "gemini_error",
		Param:   geminiError.ErrorInfo.Status,
		Code:    geminiError.ErrorInfo.Code,
	}
}

func cleaningError(errorInfo *GeminiError, key string) {
	if key == "" {
		return
	}
	message := strings.Replace(errorInfo.Message, key, "xxxxx", 1)

	// 截断 base64 数据，避免日志过长
	message = truncateBase64InMessage(message)

	errorInfo.Message = message
}

// truncateBase64InMessage 截断错误消息中的 base64 数据
func truncateBase64InMessage(message string) string {
	// 匹配 base64 数据的模式，例如: "data:image/jpeg;base64,iVBORw0KGgo..."
	// 或者直接的 base64 字符串
	const maxBase64Length = 50 // 只保留前50个字符

	// 处理 data URI 格式的 base64
	if idx := strings.Index(message, ";base64,"); idx != -1 {
		start := idx + 8 // ";base64," 的长度
		// 查找 base64 数据的结束位置（通常是引号、空格或其他分隔符）
		end := start
		for end < len(message) && isBase64Char(message[end]) {
			end++
		}

		if end-start > maxBase64Length {
			// 截断 base64 数据
			truncated := message[:start+maxBase64Length] + "...[truncated]" + message[end:]
			return truncated
		}
	}

	return message
}

// isBase64Char 检查字符是否是 base64 字符
func isBase64Char(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '='
}

func (p *GeminiProvider) GetFullRequestURL(requestURL string, modelName string) string {
	baseURL := strings.TrimSuffix(p.GetBaseURL(), "/")
	version := "v1beta"

	if p.Channel.Other != "" {
		version = p.Channel.Other
	}

	inputVersion := p.Context.Param("version")
	if inputVersion != "" {
		version = inputVersion
	}

	return fmt.Sprintf("%s/%s/models/%s:%s", baseURL, version, modelName, requestURL)

}

// 获取请求头
func (p *GeminiProvider) GetRequestHeaders() (headers map[string]string) {
	headers = make(map[string]string)
	p.CommonRequestHeaders(headers)
	headers["x-goog-api-key"] = p.Channel.Key

	return headers
}
