package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 拉取的安全与礼貌限制（需求 1.5）。
const (
	MaxBytes     = 2 << 20 // 单个响应最多 2 MB
	Timeout      = 10 * time.Second
	MaxRedirects = 3
	MaxBackoff   = 4 * time.Hour
)

// ProxyFunc 与 http.Transport.Proxy 相同。
type ProxyFunc func(*http.Request) (*url.URL, error)

// NoProxy 不使用代理。
func NoProxy(*http.Request) (*url.URL, error) { return nil, nil }

// NewClient 创建拉取用的 HTTP 客户端。base 非空时直接使用（测试注入），否则按 proxy 新建 Transport。
func NewClient(proxy ProxyFunc, base http.RoundTripper) *http.Client {
	if base == nil {
		if proxy == nil {
			proxy = NoProxy
		}
		base = &http.Transport{
			Proxy:                 proxy,
			DialContext:           (&net.Dialer{Timeout: Timeout}).DialContext,
			TLSHandshakeTimeout:   Timeout,
			ResponseHeaderTimeout: Timeout,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       90 * time.Second,
		}
	}
	return &http.Client{
		Transport: base,
		Timeout:   Timeout + 5*time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return &Error{Kind: KindRedirect, Msg: "重定向次数过多"}
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return &Error{Kind: KindURL, Msg: "重定向到了不支持的地址"}
			}
			return nil
		},
	}
}

// NormalizeURL 校验并规范化订阅地址：只允许 http / https；没写协议时补 https://。
func NormalizeURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", &Error{Kind: KindURL, Msg: "请输入订阅地址"}
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", &Error{Kind: KindURL, Msg: "地址格式不正确"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", &Error{Kind: KindURL, Msg: "只支持 http / https 地址"}
	}
	u.Fragment = ""
	return u.String(), nil
}

// SafeLink 判断条目链接能否交给浏览器打开（只允许 http / https）。
func SafeLink(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Result 是一次拉取的结果。
type Result struct {
	NotModified  bool // 304：自上次以来没有更新
	Feed         Feed
	ETag         string
	LastModified string
}

// Fetch 拉取并解析一个订阅源。etag / lastModified 来自上次成功拉取，用于条件请求（没更新时对方只回 304）。
func Fetch(ctx context.Context, c *http.Client, rawURL, userAgent, etag, lastModified string) (Result, error) {
	u, err := NormalizeURL(rawURL)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Result{}, &Error{Kind: KindURL, Msg: "地址格式不正确"}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := c.Do(req)
	if err != nil {
		return Result{}, classify(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return Result{NotModified: true, ETag: etag, LastModified: lastModified}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Result{}, &Error{Kind: KindHTTP, Status: resp.StatusCode, Msg: fmt.Sprintf("服务器返回 HTTP %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return Result{}, classify(err)
	}
	if len(body) > MaxBytes {
		return Result{}, &Error{Kind: KindTooLarge, Msg: "内容超过 2 MB，已放弃"}
	}
	f, err := ParseBytes(body)
	if err != nil {
		return Result{}, &Error{Kind: KindFormat, Msg: ErrFormat.Error()}
	}
	return Result{Feed: f, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}, nil
}

// 错误种类：界面据此给出可操作的提示。
const (
	KindURL      = "url"
	KindTimeout  = "timeout"
	KindDNS      = "dns"
	KindConnect  = "connect"
	KindTLS      = "tls"
	KindProxy    = "proxy"
	KindHTTP     = "http"
	KindFormat   = "format"
	KindTooLarge = "toolarge"
	KindRedirect = "redirect"
	KindOther    = "other"
)

// Error 是带分类的拉取错误。
type Error struct {
	Kind   string
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func classify(err error) error {
	var fe *Error
	if errors.As(err, &fe) {
		return fe
	}
	msg := err.Error()
	var dnsErr *net.DNSError
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return &Error{Kind: KindTimeout, Msg: "连接超时（网络不通或被拦截）"}
	case errors.As(err, &dnsErr):
		return &Error{Kind: KindDNS, Msg: "无法解析域名（网络不通或地址写错）"}
	case strings.Contains(msg, "proxyconnect") || strings.Contains(msg, "Proxy"):
		return &Error{Kind: KindProxy, Msg: "代理连接失败，请检查代理设置"}
	case strings.Contains(msg, "x509") || strings.Contains(msg, "tls:") || strings.Contains(msg, "certificate"):
		return &Error{Kind: KindTLS, Msg: "安全连接（证书）校验失败"}
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "actively refused"):
		return &Error{Kind: KindConnect, Msg: "连接被拒绝"}
	case strings.Contains(msg, "connection reset") || strings.Contains(msg, "forcibly closed"):
		return &Error{Kind: KindConnect, Msg: "连接被重置（可能被网络拦截）"}
	}
	return &Error{Kind: KindOther, Msg: "网络错误：" + msg}
}

// Describe 把任意错误转成给用户看的一句话。
func Describe(err error) string {
	if err == nil {
		return ""
	}
	return classify(err).Error()
}

// NextFetch 计算下次拉取时间：成功按间隔；连续失败按 1、2、4… 倍间隔退避，最长 4 小时。
func NextFetch(now time.Time, interval time.Duration, fails int) time.Time {
	d := interval
	for i := 1; i < fails && d < MaxBackoff; i++ {
		d *= 2
	}
	if d > MaxBackoff {
		d = MaxBackoff
	}
	return now.Add(d)
}
