package geminicli

import (
	"bytes"
	"done-hub/common"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/providers/gemini"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// CreateGeminiChat 创建Gemini格式的聊天（非流式）
func (p *GeminiCliProvider) CreateGeminiChat(request *gemini.GeminiChatRequest) (*gemini.GeminiChatResponse, *types.OpenAIErrorWithStatusCode) {
	req, errWithCode := p.getChatRequest(request, false)
	if errWithCode != nil {
		return nil, errWithCode
	}
	defer req.Body.Close()

	// 使用包装的响应结构
	cliResponse := &GeminiCliResponse{}
	// 发送请求
	_, errWithCode = p.Requester.SendRequest(req, cliResponse, false)
	if errWithCode != nil {
		return nil, errWithCode
	}

	// 提取实际的 Gemini 响应
	if cliResponse.Response == nil {
		return nil, common.StringErrorWrapper("no response in upstream response", "no_response", http.StatusInternalServerError)
	}

	geminiResponse := cliResponse.Response

	// 只有非 countTokens 请求才检查 candidates
	if request.Action != "countTokens" && len(geminiResponse.Candidates) == 0 {
		return nil, common.StringErrorWrapper("no candidates", "no_candidates", http.StatusInternalServerError)
	}

	usage := p.GetUsage()
	*usage = gemini.ConvertOpenAIUsage(geminiResponse.UsageMetadata)

	return geminiResponse, nil
}

// CreateGeminiChatStream 创建Gemini格式的聊天（流式）
func (p *GeminiCliProvider) CreateGeminiChatStream(request *gemini.GeminiChatRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode) {
	req, errWithCode := p.getChatRequest(request, true)
	if errWithCode != nil {
		return nil, errWithCode
	}
	defer req.Body.Close()

	channel := p.GetChannel()

	// 使用 GeminiCli 专用的 Relay 流处理器
	chatHandler := &GeminiCliRelayStreamHandler{
		Usage:     p.Usage,
		ModelName: request.Model,
		Prefix:    `data: `,
		Key:       channel.Key,
	}

	// 发送请求
	resp, errWithCode := p.Requester.SendRequestRaw(req)
	if errWithCode != nil {
		return nil, errWithCode
	}

	stream, errWithCode := requester.RequestNoTrimStream(p.Requester, resp, chatHandler.HandlerStream)
	if errWithCode != nil {
		return nil, errWithCode
	}

	return stream, nil
}

// GeminiCliRelayStreamHandler GeminiCli Relay 流式响应处理器
type GeminiCliRelayStreamHandler struct {
	Usage     *types.Usage
	Prefix    string
	ModelName string
	Key       string
}

// HandlerStream 处理流式响应
func (h *GeminiCliRelayStreamHandler) HandlerStream(rawLine *[]byte, dataChan chan string, errChan chan error) {
	rawStr := string(*rawLine)

	// 如果不是 data: 开头，直接转发
	if !strings.HasPrefix(rawStr, h.Prefix) {
		dataChan <- rawStr
		return
	}

	// 去除 "data: " 前缀
	noSpaceLine := bytes.TrimSpace(*rawLine)
	noSpaceLine = noSpaceLine[6:] // 去除 "data: "

	// 解析包装的响应
	var cliResponse GeminiCliResponse
	err := json.Unmarshal(noSpaceLine, &cliResponse)
	if err != nil {
		logger.SysError(fmt.Sprintf("Failed to unmarshal GeminiCli relay stream response: %s", err.Error()))
		errChan <- gemini.ErrorToGeminiErr(err)
		return
	}

	// 提取实际的 Gemini 响应
	if cliResponse.Response == nil {
		logger.SysError("GeminiCli relay stream response has no 'response' field")
		return
	}

	geminiResponse := cliResponse.Response

	// 检查错误
	if geminiResponse.ErrorInfo != nil {
		cleaningError(geminiResponse.ErrorInfo, h.Key)
		errChan <- geminiResponse.ErrorInfo
		return
	}

	// 更新 usage
	if geminiResponse.UsageMetadata != nil {
		h.Usage.PromptTokens = geminiResponse.UsageMetadata.PromptTokenCount

		// 计算 completion tokens，确保不为负数
		completionTokens := geminiResponse.UsageMetadata.CandidatesTokenCount + geminiResponse.UsageMetadata.ThoughtsTokenCount
		if completionTokens < 0 {
			completionTokens = 0
		}
		h.Usage.CompletionTokens = completionTokens
		h.Usage.CompletionTokensDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount

		// 如果 TotalTokenCount 为 0 但有 PromptTokenCount，则计算总数
		totalTokens := geminiResponse.UsageMetadata.TotalTokenCount
		if totalTokens == 0 && geminiResponse.UsageMetadata.PromptTokenCount > 0 {
			totalTokens = geminiResponse.UsageMetadata.PromptTokenCount + completionTokens
		}
		h.Usage.TotalTokens = totalTokens
	}

	// 重新序列化实际的 Gemini 响应并转发
	responseJSON, err := json.Marshal(geminiResponse)
	if err != nil {
		logger.SysError(fmt.Sprintf("Failed to marshal Gemini response: %s", err.Error()))
		errChan <- gemini.ErrorToGeminiErr(err)
		return
	}

	// 转发为 data: {...} 格式
	dataChan <- fmt.Sprintf("data: %s", string(responseJSON))
}
