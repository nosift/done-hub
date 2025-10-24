package requester

import (
	"done-hub/common/utils"
	"net/http"
	"time"
)

var HTTPClient *http.Client

func InitHttpClient() {
	trans := &http.Transport{
		DialContext: utils.Socks5ProxyFunc,
		Proxy:       utils.ProxyFunc,

		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 200,
		MaxConnsPerHost:     500,
		IdleConnTimeout:     90 * time.Second,

		// 超时配置 - 针对 SSE 长连接优化
		TLSHandshakeTimeout:   10 * time.Second,
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
