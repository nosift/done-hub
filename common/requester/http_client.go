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

		MaxIdleConns:        300,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     120 * time.Second,

		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,

		DisableKeepAlives:  false, // 启用 Keep-Alive（复用连接）
		DisableCompression: false, // 启用压缩
		ForceAttemptHTTP2:  true,  // 优先使用 HTTP/2（更高效）
	}

	HTTPClient = &http.Client{
		Transport: trans,
	}

	relayTimeout := utils.GetOrDefault("relay_timeout", 0)
	if relayTimeout > 0 {
		HTTPClient.Timeout = time.Duration(relayTimeout) * time.Second
	}
}
