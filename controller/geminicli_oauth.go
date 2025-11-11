package controller

import (
	"crypto/rand"
	"done-hub/common"
	"done-hub/common/cache"
	"done-hub/common/logger"
	"done-hub/providers/geminicli"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// OAuth 状态缓存前缀
	OAuthStateCachePrefix = "geminicli_oauth_state:"
	// OAuth 结果缓存前缀
	OAuthResultCachePrefix = "geminicli_oauth_result:"
	// OAuth 状态缓存时长（30分钟）
	OAuthStateCacheDuration = 30 * time.Minute
	// OAuth 结果缓存时长（10分钟）
	OAuthResultCacheDuration = 10 * time.Minute
)

// OAuthStateData OAuth 状态数据
type OAuthStateData struct {
	ChannelID int    `json:"channel_id"`
	ProjectID string `json:"project_id"`
	CreatedAt int64  `json:"created_at"`
}

// OAuthResultData OAuth 结果数据
type OAuthResultData struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	ProjectID   string `json:"project_id"`
	Credentials string `json:"credentials"`
	CompletedAt int64  `json:"completed_at"`
}

// StartGeminiCliOAuthRequest 开始 OAuth 认证请求
type StartGeminiCliOAuthRequest struct {
	ChannelID int    `json:"channel_id"` // 可选，新建时为 0
	ProjectID string `json:"project_id"` // 可选，为空时自动检测
}

// StartGeminiCliOAuth 开始 GeminiCli OAuth 认证流程
// POST /api/geminicli/oauth/start
func StartGeminiCliOAuth(c *gin.Context) {
	var req StartGeminiCliOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}

	// 生成随机 state
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("failed to generate state: %w", err))
		return
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// 保存 state 到缓存
	stateData := OAuthStateData{
		ChannelID: req.ChannelID,
		ProjectID: req.ProjectID,
		CreatedAt: time.Now().Unix(),
	}
	cacheKey := OAuthStateCachePrefix + state
	cache.SetCache(cacheKey, stateData, OAuthStateCacheDuration)

	// 构建 OAuth 授权 URL
	redirectURI := "http://localhost:8080/api/geminicli/oauth/callback"

	params := url.Values{}
	params.Set("client_id", geminicli.DefaultClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join([]string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
	}, " "))
	params.Set("response_type", "code")
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("include_granted_scopes", "true")
	params.Set("state", state)

	// 将 project_id 添加到授权 URL 参数中，以便 Google 知道要授权哪个项目
	if req.ProjectID != "" {
		params.Set("project_id", req.ProjectID)
	}

	authURL := "https://accounts.google.com/o/oauth2/auth?" + params.Encode()

	message := "请在浏览器中访问 auth_url 完成授权"
	autoDetect := false
	if req.ProjectID == "" {
		message = "请在浏览器中访问 auth_url 完成授权，授权完成后将自动检测项目 ID"
		autoDetect = true
	}

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"auth_url":            authURL,
		"state":               state,
		"message":             message,
		"auto_project_detect": autoDetect,
		"detected_project_id": req.ProjectID,
	})
}

// GetGeminiCliOAuthStatus 查询 OAuth 授权状态
// GET /api/geminicli/oauth/status/:state
func GetGeminiCliOAuthStatus(c *gin.Context) {
	state := c.Param("state")
	if state == "" {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("state parameter is required"))
		return
	}

	// 从缓存获取结果
	resultCacheKey := OAuthResultCachePrefix + state
	result, err := cache.GetCache[OAuthResultData](resultCacheKey)
	if err != nil {
		// 还没有结果，返回 pending 状态
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"status":  "pending",
			"message": "授权进行中，请完成授权流程",
		})
		return
	}

	// 返回结果
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"status":      "completed",
		"result":      result.Success,
		"message":     result.Message,
		"project_id":  result.ProjectID,
		"credentials": result.Credentials,
	})
}

// GeminiCliOAuthCallback OAuth 回调处理
// GET /api/geminicli/oauth/callback
func GeminiCliOAuthCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")

	// 检查是否有错误
	if errorParam != "" {
		errorDesc := c.Query("error_description")
		logger.SysError(fmt.Sprintf("OAuth callback error: %s - %s", errorParam, errorDesc))

		// 保存错误结果到缓存
		if state != "" {
			resultCacheKey := OAuthResultCachePrefix + state
			resultData := OAuthResultData{
				Success:     false,
				Message:     fmt.Sprintf("授权失败: %s", errorDesc),
				CompletedAt: time.Now().Unix(),
			}
			cache.SetCache(resultCacheKey, resultData, OAuthResultCacheDuration)
		}

		renderOAuthResult(c, false, fmt.Sprintf("授权失败: %s", errorDesc), "", "", state)
		return
	}

	// 验证 state
	if state == "" {
		renderOAuthResult(c, false, "无效的 state 参数", "", "", "")
		return
	}

	// 从缓存获取 state 数据
	cacheKey := OAuthStateCachePrefix + state
	stateData, err := cache.GetCache[OAuthStateData](cacheKey)
	if err != nil {
		logger.SysError(fmt.Sprintf("Failed to get OAuth state from cache: %s", err.Error()))

		// 保存错误结果到缓存
		resultCacheKey := OAuthResultCachePrefix + state
		resultData := OAuthResultData{
			Success:     false,
			Message:     "OAuth 状态已过期或无效，请重新开始授权流程",
			CompletedAt: time.Now().Unix(),
		}
		cache.SetCache(resultCacheKey, resultData, OAuthResultCacheDuration)

		renderOAuthResult(c, false, "OAuth 状态已过期或无效，请重新开始授权流程", "", "", state)
		return
	}

	// 删除已使用的 state
	cache.DeleteCache(cacheKey)

	// 使用 code 交换 token
	redirectURI := "http://localhost:8080/api/geminicli/oauth/callback"

	tokenResp, err := exchangeCodeForToken(code, redirectURI)
	if err != nil {
		logger.SysError(fmt.Sprintf("Failed to exchange code for token: %s", err.Error()))

		// 保存错误结果到缓存
		resultCacheKey := OAuthResultCachePrefix + state
		resultData := OAuthResultData{
			Success:     false,
			Message:     fmt.Sprintf("获取 token 失败: %s", err.Error()),
			CompletedAt: time.Now().Unix(),
		}
		cache.SetCache(resultCacheKey, resultData, OAuthResultCacheDuration)

		renderOAuthResult(c, false, fmt.Sprintf("获取 token 失败: %s", err.Error()), "", "", state)
		return
	}

	// 确定项目 ID
	projectID := stateData.ProjectID
	autoDetected := false

	// 如果没有提供项目 ID，尝试自动检测
	if projectID == "" {
		logger.SysLog("Project ID 未提供，尝试自动检测...")
		projects, err := getUserProjects(tokenResp.AccessToken)
		if err != nil {
			logger.SysError(fmt.Sprintf("Failed to get user projects: %s", err.Error()))

			// 保存错误结果到缓存
			resultCacheKey := OAuthResultCachePrefix + state
			resultData := OAuthResultData{
				Success:     false,
				Message:     fmt.Sprintf("自动检测项目 ID 失败: %s。请手动填写 Project ID 后重新授权", err.Error()),
				CompletedAt: time.Now().Unix(),
			}
			cache.SetCache(resultCacheKey, resultData, OAuthResultCacheDuration)

			renderOAuthResult(c, false, fmt.Sprintf("自动检测项目 ID 失败: %s。请手动填写 Project ID 后重新授权", err.Error()), "", "", state)
			return
		}

		if len(projects) == 0 {
			logger.SysError("No accessible projects found")

			// 保存错误结果到缓存
			resultCacheKey := OAuthResultCachePrefix + state
			resultData := OAuthResultData{
				Success:     false,
				Message:     "未检测到可访问的项目，请检查权限或手动填写 Project ID 后重新授权",
				CompletedAt: time.Now().Unix(),
			}
			cache.SetCache(resultCacheKey, resultData, OAuthResultCacheDuration)

			renderOAuthResult(c, false, "未检测到可访问的项目，请检查权限或手动填写 Project ID 后重新授权", "", "", state)
			return
		}

		// 选择默认项目
		projectID = selectDefaultProject(projects)
		autoDetected = true
		logger.SysLog(fmt.Sprintf("自动检测到项目 ID: %s (共 %d 个可用项目)", projectID, len(projects)))
	}

	// 构建完整的凭证
	creds := geminicli.OAuth2Credentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ClientID:     geminicli.DefaultClientID,
		ClientSecret: geminicli.DefaultClientSecret,
		ProjectID:    projectID,
		TokenType:    tokenResp.TokenType,
	}

	// 计算过期时间
	if tokenResp.ExpiresIn > 0 {
		creds.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	// 序列化凭证为 JSON
	credsJSON, err := creds.ToJSON()
	if err != nil {
		logger.SysError(fmt.Sprintf("Failed to serialize credentials: %s", err.Error()))

		// 保存错误结果到缓存
		resultCacheKey := OAuthResultCachePrefix + state
		resultData := OAuthResultData{
			Success:     false,
			Message:     "凭证序列化失败",
			CompletedAt: time.Now().Unix(),
		}
		cache.SetCache(resultCacheKey, resultData, OAuthResultCacheDuration)

		renderOAuthResult(c, false, "凭证序列化失败", "", "", state)
		return
	}

	// 自动启用必需的 API 服务（异步执行，不阻塞响应）
	go func() {
		if err := enableRequiredAPIs(tokenResp.AccessToken, projectID); err != nil {
			logger.SysError(fmt.Sprintf("Failed to enable required APIs for project %s: %s", projectID, err.Error()))
		}
	}()

	// 构建成功消息
	successMessage := "授权成功"
	if autoDetected {
		successMessage = fmt.Sprintf("授权成功！已自动检测并使用项目 ID: %s", projectID)
	}

	// 保存结果到缓存，供前端轮询
	resultCacheKey := OAuthResultCachePrefix + state
	resultData := OAuthResultData{
		Success:     true,
		Message:     successMessage,
		ProjectID:   projectID,
		Credentials: credsJSON,
		CompletedAt: time.Now().Unix(),
	}
	cache.SetCache(resultCacheKey, resultData, OAuthResultCacheDuration)

	// 返回 HTML 页面，通过 postMessage 发送凭证给父窗口（如果有的话）
	renderOAuthResult(c, true, successMessage, projectID, credsJSON, state)
}

// renderOAuthResult 渲染 OAuth 结果页面
func renderOAuthResult(c *gin.Context, success bool, message, projectID, credentials, state string) {
	successStr := "true"
	credentialsJSON := "null"
	statusClass := "success"
	statusText := "授权成功"
	iconSVG := `<svg width="80" height="80" viewBox="0 0 80 80" fill="none" xmlns="http://www.w3.org/2000/svg">
		<circle cx="40" cy="40" r="36" fill="#34C759" fill-opacity="0.1"/>
		<circle cx="40" cy="40" r="32" fill="#34C759"/>
		<path d="M25 40L35 50L55 30" stroke="white" stroke-width="4" stroke-linecap="round" stroke-linejoin="round"/>
	</svg>`
	detailMessage := message

	if !success {
		statusClass = "error"
		statusText = "授权失败"
		successStr = "false"
		iconSVG = `<svg width="80" height="80" viewBox="0 0 80 80" fill="none" xmlns="http://www.w3.org/2000/svg">
			<circle cx="40" cy="40" r="36" fill="#FF3B30" fill-opacity="0.1"/>
			<circle cx="40" cy="40" r="32" fill="#FF3B30"/>
			<path d="M30 30L50 50M50 30L30 50" stroke="white" stroke-width="4" stroke-linecap="round"/>
		</svg>`
	} else if credentials != "" {
		// 转义 JSON 字符串中的特殊字符
		escapedCreds := strings.ReplaceAll(credentials, `\`, `\\`)
		escapedCreds = strings.ReplaceAll(escapedCreds, `"`, `\"`)
		escapedCreds = strings.ReplaceAll(escapedCreds, "\n", `\n`)
		credentialsJSON = fmt.Sprintf(`"%s"`, escapedCreds)
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>OAuth 授权</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            background: #f5f5f7;
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
        }

        .container {
            background: white;
            padding: 48px 40px;
            border-radius: 18px;
            box-shadow: 0 4px 24px rgba(0, 0, 0, 0.06);
            text-align: center;
            max-width: 400px;
            width: 90%%;
            animation: slideUp 0.4s cubic-bezier(0.16, 1, 0.3, 1);
        }

        @keyframes slideUp {
            from {
                opacity: 0;
                transform: translateY(20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        .icon {
            margin-bottom: 24px;
            animation: scaleIn 0.5s cubic-bezier(0.16, 1, 0.3, 1) 0.1s both;
        }

        @keyframes scaleIn {
            from {
                opacity: 0;
                transform: scale(0.8);
            }
            to {
                opacity: 1;
                transform: scale(1);
            }
        }

        h1 {
            font-size: 28px;
            font-weight: 600;
            color: #1d1d1f;
            margin-bottom: 12px;
            letter-spacing: -0.5px;
        }

        .message {
            font-size: 17px;
            color: #86868b;
            margin-bottom: 32px;
            line-height: 1.5;
        }

        .countdown {
            font-size: 15px;
            color: #86868b;
            margin-bottom: 24px;
        }

        .close-btn {
            width: 100%%;
            padding: 14px 24px;
            background: #007AFF;
            color: white;
            border: none;
            border-radius: 12px;
            cursor: pointer;
            font-size: 17px;
            font-weight: 500;
            transition: all 0.2s ease;
            -webkit-tap-highlight-color: transparent;
        }

        .close-btn:hover {
            background: #0051D5;
            transform: scale(0.98);
        }

        .close-btn:active {
            transform: scale(0.96);
        }

        .success h1 {
            color: #34C759;
        }

        .error h1 {
            color: #FF3B30;
        }
    </style>
</head>
<body>
    <div class="container %s">
        <div class="icon">%s</div>
        <h1>%s</h1>
        <p class="message">%s</p>
        <p class="countdown" id="countdown">窗口将在 3 秒后自动关闭</p>
        <button class="close-btn" onclick="closeWindow()">关闭窗口</button>
    </div>
    <script>
        console.log('OAuth callback page loaded');
        console.log('Success:', %s);
        console.log('ProjectID:', '%s');
        console.log('Credentials:', %s);

        // 发送消息给父窗口
        if (window.opener && !window.opener.closed) {
            console.log('Sending message to parent window');
            window.opener.postMessage({
                type: 'geminicli_oauth_result',
                success: %s,
                projectId: '%s',
                credentials: %s
            }, '*');
            console.log('Message sent');
        } else {
            console.error('No opener window found');
        }

        function closeWindow() {
            window.close();
            setTimeout(function() {
                if (!window.closed) {
                    document.getElementById('countdown').innerHTML = '请手动关闭此窗口';
                }
            }, 100);
        }

        // 3秒倒计时后自动关闭
        var countdown = 3;
        var countdownEl = document.getElementById('countdown');
        var countdownInterval = setInterval(function() {
            countdown--;
            if (countdown > 0) {
                countdownEl.innerHTML = '窗口将在 ' + countdown + ' 秒后自动关闭';
            } else {
                clearInterval(countdownInterval);
                console.log('Auto closing window');
                window.close();
                setTimeout(function() {
                    if (!window.closed) {
                        countdownEl.innerHTML = '请手动关闭此窗口';
                    }
                }, 100);
            }
        }, 1000);
    </script>
</body>
</html>
`, statusClass, iconSVG, statusText, detailMessage, successStr, projectID, credentialsJSON, successStr, projectID, credentialsJSON)

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// enableRequiredAPIs 启用必需的 API 服务
func enableRequiredAPIs(accessToken, projectID string) error {
	requiredServices := []string{
		"geminicloudassist.googleapis.com", // Gemini Cloud Assist API
		"cloudaicompanion.googleapis.com",  // Gemini for Google Cloud API
	}

	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
		"User-Agent":    "done-hub-geminicli/1.0",
	}

	for _, service := range requiredServices {
		// 检查服务是否已启用
		checkURL := fmt.Sprintf("https://serviceusage.googleapis.com/v1/projects/%s/services/%s", projectID, service)
		req, err := http.NewRequest("GET", checkURL, nil)
		if err != nil {
			logger.SysError(fmt.Sprintf("Failed to create check request for %s: %s", service, err.Error()))
			continue
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			logger.SysError(fmt.Sprintf("Failed to check service %s: %s", service, err.Error()))
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				var serviceData map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&serviceData); err == nil {
					if state, ok := serviceData["state"].(string); ok && state == "ENABLED" {
						continue
					}
				}
			}
		}

		// 启用服务
		enableURL := fmt.Sprintf("https://serviceusage.googleapis.com/v1/projects/%s/services/%s:enable", projectID, service)
		req, err = http.NewRequest("POST", enableURL, strings.NewReader("{}"))
		if err != nil {
			logger.SysError(fmt.Sprintf("Failed to create enable request for %s: %s", service, err.Error()))
			continue
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err = client.Do(req)
		if err != nil {
			logger.SysError(fmt.Sprintf("Failed to enable service %s: %s", service, err.Error()))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 400 {
			body, _ := io.ReadAll(resp.Body)
			logger.SysError(fmt.Sprintf("Failed to enable service %s: %d - %s", service, resp.StatusCode, string(body)))
		}
	}

	return nil
}

// exchangeCodeForToken 使用授权码交换 token
func exchangeCodeForToken(code, redirectURI string) (*geminicli.TokenRefreshResponse, error) {
	data := url.Values{}
	data.Set("client_id", geminicli.DefaultClientID)
	data.Set("client_secret", geminicli.DefaultClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequest("POST", geminicli.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp geminicli.TokenRefreshError
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil {
			return nil, fmt.Errorf("token exchange failed: %s - %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp geminicli.TokenRefreshResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &tokenResp, nil
}

// GoogleCloudProject Google Cloud 项目信息
type GoogleCloudProject struct {
	ProjectID      string `json:"projectId"`
	ProjectNumber  string `json:"projectNumber"`
	DisplayName    string `json:"name"`
	LifecycleState string `json:"lifecycleState"`
}

// GoogleCloudProjectsResponse Google Cloud 项目列表响应
type GoogleCloudProjectsResponse struct {
	Projects []GoogleCloudProject `json:"projects"`
}

// getUserProjects 获取用户可访问的 Google Cloud 项目列表
func getUserProjects(accessToken string) ([]GoogleCloudProject, error) {
	req, err := http.NewRequest("GET", "https://cloudresourcemanager.googleapis.com/v1/projects", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "geminicli-oauth/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get projects: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var projectsResp GoogleCloudProjectsResponse
	if err := json.Unmarshal(bodyBytes, &projectsResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// 只返回活跃的项目
	activeProjects := make([]GoogleCloudProject, 0)
	for _, project := range projectsResp.Projects {
		if project.LifecycleState == "ACTIVE" {
			activeProjects = append(activeProjects, project)
		}
	}

	return activeProjects, nil
}

// selectDefaultProject 从项目列表中选择默认项目
func selectDefaultProject(projects []GoogleCloudProject) string {
	if len(projects) == 0 {
		return ""
	}

	// 策略1：查找包含 "default" 的项目
	for _, project := range projects {
		if strings.Contains(strings.ToLower(project.DisplayName), "default") ||
			strings.Contains(strings.ToLower(project.ProjectID), "default") {
			logger.SysLog(fmt.Sprintf("选择默认项目: %s (%s)", project.ProjectID, project.DisplayName))
			return project.ProjectID
		}
	}

	// 策略2：选择第一个项目
	firstProject := projects[0]
	logger.SysLog(fmt.Sprintf("选择第一个项目作为默认: %s (%s)", firstProject.ProjectID, firstProject.DisplayName))
	return firstProject.ProjectID
}
