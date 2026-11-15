package feed

import (
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// WinProxy 根据 Windows「当前用户代理设置」（WinHTTP 读到的 Proxy / ProxyBypass 字符串）生成代理函数。
//
//	proxy:  "host:port"，或按协议分开写的 "http=h1:p1;https=h2:p2;socks=…"（可带 http:// 前缀）
//	bypass: 以分号或空白分隔，支持 "<local>"（不含点的主机名）、"*.corp.com"、"10.*" 这样的通配
//
// 本机地址（localhost / 127.x / ::1）总是直连。PAC 自动配置脚本不在这里处理。
func WinProxy(proxy, bypass string) ProxyFunc {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return NoProxy
	}
	byScheme := map[string]string{}
	var all string
	for _, part := range strings.FieldsFunc(proxy, func(r rune) bool { return r == ';' || r == ' ' }) {
		if k, v, ok := strings.Cut(part, "="); ok {
			byScheme[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		} else if all == "" {
			all = strings.TrimSpace(part)
		}
	}
	var rules []string
	for _, b := range strings.FieldsFunc(bypass, func(r rune) bool { return r == ';' || r == ' ' || r == ',' || r == '\t' }) {
		rules = append(rules, strings.ToLower(strings.TrimSpace(b)))
	}
	return func(req *http.Request) (*url.URL, error) {
		host := strings.ToLower(req.URL.Hostname())
		if bypassed(host, rules) {
			return nil, nil
		}
		p := all
		if v := byScheme[req.URL.Scheme]; v != "" {
			p = v
		} else if v := byScheme["http"]; v != "" && p == "" {
			p = v // 只配置了 http 代理时，https 也走它（系统「对所有协议使用相同代理」的常见写法）
		}
		if p == "" {
			return nil, nil
		}
		if !strings.Contains(p, "://") {
			p = "http://" + p
		}
		return url.Parse(p)
	}
}

func bypassed(host string, rules []string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, r := range rules {
		switch {
		case r == "<local>":
			if !strings.Contains(host, ".") {
				return true
			}
		case r == host:
			return true
		default:
			r = strings.TrimPrefix(strings.TrimPrefix(r, "http://"), "https://")
			if ok, _ := path.Match(r, host); ok {
				return true
			}
		}
	}
	return false
}

// ManualProxy 由用户手动填写的代理地址生成代理函数（空串 = 不使用代理）。
func ManualProxy(addr string) (ProxyFunc, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return NoProxy, nil
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return nil, &Error{Kind: KindProxy, Msg: "代理地址格式不正确（例如 127.0.0.1:7890）"}
	}
	return http.ProxyURL(u), nil
}
