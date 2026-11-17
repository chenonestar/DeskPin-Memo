package feed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const rss2 = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel><title>少数派</title><atom:link href="https://sspai.com/feed" rel="self"/>
<item><title>第一条&nbsp;新闻</title><link>https://sspai.com/post/1</link><atom:link href="https://x/y" rel="self"/>
<guid isPermaLink="false">p1</guid><pubDate>Sat, 14 Nov 2026 08:30:00 +0800</pubDate>
<description><![CDATA[<p>摘要 <img src="http://evil/x.png"><script>alert(1)</script>正文</p>]]></description></item>
<item><title>没有链接的条目</title></item>
<item><title>只有 content:encoded</title><link>https://sspai.com/post/2</link><pubDate>Sat, 14 Nov 2026 09:00:00 CST</pubDate>
<content:encoded><![CDATA[<b>粗体</b>内容]]></content:encoded></item>
</channel></rss>`

const atom = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>阮一峰的网络日志</title>
<entry><title type="html">科技爱好者周刊 &amp;lt;第 1 期&amp;gt;</title><link rel="alternate" type="text/html" href="https://www.ruanyifeng.com/blog/1.html"/>
<link rel="replies" href="https://x/comments"/><id>tag:ruanyifeng.com,2026:/blog//1.1</id>
<published>2026-11-13T00:00:00Z</published><updated>2026-11-14T00:00:00Z</updated><summary>这里记录每周值得分享的科技内容</summary></entry>
</feed>`

const rdf = `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel><title>Solidot</title></channel>
<item><title>RDF 条目</title><link>https://www.solidot.org/story?sid=1</link><dc:date>2026-11-14T10:00:00+08:00</dc:date><description>描述</description></item>
</rdf:RDF>`

func TestParseRSS2(t *testing.T) {
	f, err := ParseBytes([]byte(rss2))
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "少数派" || len(f.Items) != 2 {
		t.Fatalf("%+v", f)
	}
	a := f.Items[0]
	if a.Title != "第一条 新闻" || a.Link != "https://sspai.com/post/1" || a.GUID != "p1" {
		t.Fatalf("%+v", a)
	}
	if strings.Contains(a.Summary, "<") || strings.Contains(a.Summary, "evil") || a.Summary != "摘要 alert(1) 正文" {
		t.Fatalf("摘要应为去掉 HTML 的纯文本: %q", a.Summary)
	}
	if !a.Published.Equal(time.Date(2026, 11, 14, 0, 30, 0, 0, time.UTC)) {
		t.Fatal(a.Published)
	}
	b := f.Items[1]
	if b.Summary != "粗体 内容" && b.Summary != "粗体内容" {
		t.Fatalf("content:encoded 作摘要: %q", b.Summary)
	}
	if !b.Published.Equal(time.Date(2026, 11, 14, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("CST 按北京时间: %v", b.Published)
	}
	if b.GUID != b.Link {
		t.Fatal("没有 guid 时用链接")
	}
}

func TestParseAtomAndRDF(t *testing.T) {
	f, err := ParseBytes([]byte(atom))
	if err != nil || f.Title != "阮一峰的网络日志" || len(f.Items) != 1 {
		t.Fatalf("%+v %v", f, err)
	}
	e := f.Items[0]
	if e.Title != "科技爱好者周刊 <第 1 期>" || e.Link != "https://www.ruanyifeng.com/blog/1.html" || e.Published.Year() != 2026 || e.Published.Day() != 13 {
		t.Fatalf("%+v", e)
	}
	r, err := ParseBytes([]byte(rdf))
	if err != nil || r.Title != "Solidot" || len(r.Items) != 1 || r.Items[0].Published.Hour() != 10 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestParseGBK(t *testing.T) {
	src := `<?xml version="1.0" encoding="gb2312"?><rss><channel><title>国内老站</title><item><title>中文标题</title><link>http://a/1</link></item></channel></rss>`
	b, _ := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(src))
	f, err := ParseBytes(b)
	if err != nil || f.Title != "国内老站" || f.Items[0].Title != "中文标题" {
		t.Fatalf("GBK 编码应正确转码: %+v %v", f, err)
	}
}

func TestParseRejectsNonFeed(t *testing.T) {
	for _, s := range []string{"<html><body>hi</body></html>", "not xml at all", "", `<?xml version="1.0"?><other/>`} {
		if _, err := ParseBytes([]byte(s)); !errors.Is(err, ErrFormat) {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestCleanTruncates(t *testing.T) {
	s := clean(strings.Repeat("字", 300), SummaryMax)
	if r := []rune(s); len(r) != SummaryMax+1 || r[len(r)-1] != '…' {
		t.Fatal(len(r))
	}
}

func TestNormalizeAndSafeLink(t *testing.T) {
	if u, err := NormalizeURL(" sspai.com/feed "); err != nil || u != "https://sspai.com/feed" {
		t.Fatal(u, err)
	}
	for _, bad := range []string{"", "file:///c:/x.xml", "javascript:alert(1)", "ftp://a/b", "https://"} {
		if _, err := NormalizeURL(bad); err == nil {
			t.Errorf("应拒绝 %q", bad)
		}
	}
	if !SafeLink("https://a.com/x") || SafeLink("javascript:alert(1)") || SafeLink("file:///etc/passwd") || SafeLink("") {
		t.Fatal("SafeLink")
	}
}

func TestFetchConditionalAndLimits(t *testing.T) {
	var gotUA, gotINM string
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotINM = r.UserAgent(), r.Header.Get("If-None-Match")
		if gotINM == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, rss2)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) { w.Write(bytes.Repeat([]byte("x"), MaxBytes+10)) })
	mux.HandleFunc("/403", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) })
	mux.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html>登录页</html>") })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	mux.HandleFunc("/toFile", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(nil, nil)
	ctx := context.Background()

	r, err := Fetch(ctx, c, srv.URL+"/ok", "DeskPinMemo/test", "", "")
	if err != nil || r.NotModified || r.ETag != `"v1"` || len(r.Feed.Items) != 2 || gotUA != "DeskPinMemo/test" {
		t.Fatalf("%+v %v %q", r, err, gotUA)
	}
	r, err = Fetch(ctx, c, srv.URL+"/ok", "ua", r.ETag, "")
	if err != nil || !r.NotModified || gotINM != `"v1"` {
		t.Fatalf("条件请求应得到 304: %+v %v", r, err)
	}
	for path, kind := range map[string]string{"/big": KindTooLarge, "/403": KindHTTP, "/html": KindFormat, "/loop": KindRedirect, "/toFile": KindURL} {
		_, err := Fetch(ctx, c, srv.URL+path, "ua", "", "")
		var fe *Error
		if !errors.As(err, &fe) || fe.Kind != kind {
			t.Errorf("%s: 期望 %s，得到 %v", path, kind, err)
		}
	}
	// 连不上：本机一个没人监听的端口
	_, err = Fetch(ctx, c, "http://127.0.0.1:1/feed", "ua", "", "")
	if d := Describe(err); !strings.Contains(d, "连接被拒绝") && !strings.Contains(d, "超时") {
		t.Fatalf("应给出可读的原因: %q", d)
	}
}

func TestNextFetchBackoff(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	iv := 30 * time.Minute
	cases := map[int]time.Duration{0: iv, 1: iv, 2: 2 * iv, 3: 4 * iv, 4: 4 * time.Hour, 10: 4 * time.Hour}
	for fails, want := range cases {
		if got := NextFetch(now, iv, fails).Sub(now); got != want {
			t.Errorf("fails=%d: %v want %v", fails, got, want)
		}
	}
}

func TestWinProxy(t *testing.T) {
	req := func(u string) *http.Request { r, _ := http.NewRequest("GET", u, nil); return r }
	p := WinProxy("proxy.corp:8080", "<local>;*.corp.com;10.*")
	if u, _ := p(req("https://sspai.com/feed")); u == nil || u.String() != "http://proxy.corp:8080" {
		t.Fatalf("%v", u)
	}
	for _, direct := range []string{"http://intranet/rss", "https://news.corp.com/x", "http://10.1.2.3/a", "http://127.0.0.1:8790/x", "http://localhost/x"} {
		if u, _ := p(req(direct)); u != nil {
			t.Errorf("%s 应直连，得到 %v", direct, u)
		}
	}
	q := WinProxy("http=h1:1;https=h2:2", "")
	if u, _ := q(req("https://a.com")); u.Host != "h2:2" {
		t.Fatal(u)
	}
	if u, _ := q(req("http://a.com")); u.Host != "h1:1" {
		t.Fatal(u)
	}
	if u, _ := WinProxy("http=h1:1", "")(req("https://a.com")); u == nil || u.Host != "h1:1" {
		t.Fatal("只有 http 代理时 https 也走它")
	}
	if u, _ := WinProxy("", "")(req("https://a.com")); u != nil {
		t.Fatal("没有代理")
	}
	if _, err := ManualProxy("::bad::"); err == nil {
		t.Fatal("应拒绝错误的代理地址")
	}
	mp, _ := ManualProxy("127.0.0.1:7890")
	if u, _ := mp(req("https://a.com")); u.String() != "http://127.0.0.1:7890" {
		t.Fatal(u)
	}
}

func TestPresetsAreValid(t *testing.T) {
	seen := map[string]bool{}
	defaults := 0
	for _, p := range Presets {
		// 只有标为待验证的老接口允许 http
		if _, err := NormalizeURL(p.URL); err != nil || (!strings.HasPrefix(p.URL, "https://") && !p.Unverified) || p.Name == "" || seen[p.URL] {
			t.Errorf("%+v", p)
		}
		seen[p.URL] = true
		if p.Default {
			defaults++
		}
	}
	if defaults != 3 {
		t.Fatalf("默认勾选 3 个: %d", defaults)
	}
	fin := 0
	for _, p := range Presets {
		if p.Category == "财经" {
			fin++
			if p.Default {
				t.Errorf("财经源默认不勾选: %s", p.Name)
			}
		}
	}
	if fin != 4 {
		t.Fatalf("财经源 4 个: %d", fin)
	}
}
