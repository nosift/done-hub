package requester

import (
	"done-hub/common/utils"
	"net/http"
	"time"
)

var HTTPClient *http.Client

func InitHttpClient() {
	// TLS 握手超时配置，默认 30 秒，可通过环境变量 TLS_HANDSHAKE_TIMEOUT 配置
	tlsHandshakeTimeout := time.Duration(utils.GetOrDefault("tls_handshake_timeout", 30)) * time.Second

	trans := &http.Transport{
		DialContext: utils.Socks5ProxyFunc,
		Proxy:       utils.ProxyFunc,

		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     60 * time.Second,

		// 超时配置 - 针对 SSE 长连接优化
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 0,

		// 连接复用优化
		DisableKeepAlives:  false,
		DisableCompression: false,
		ForceAttemptHTTP2:  true,
	}

	HTTPClient = &http.Client{
		Transport: trans,
		Timeout:   0,
	}

	relayTimeout := utils.GetOrDefault("relay_timeout", 0)
	if relayTimeout > 0 {
		HTTPClient.Timeout = time.Duration(relayTimeout) * time.Second
	}
}
