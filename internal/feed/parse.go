// Package feed 负责 RSS / Atom 订阅源的解析与拉取（摸鱼功能 FR-701~707）。
//
// 只做三件事：把订阅地址的 XML 解析成统一的条目列表、按礼貌规则拉取、把错误翻译成人能看懂的话。
// 不涉及存储与调度（见 service）。与平台无关，全部可在任意系统上测试。
package feed

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"html"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

// Feed 是解析后的订阅源。
type Feed struct {
	Title string
	Items []Item
}

// Item 是一条新闻。
type Item struct {
	GUID      string
	Title     string
	Link      string
	Summary   string    // 纯文本摘要，最多 SummaryMax 个字
	Published time.Time // 解析不出时为零值，由调用方用拉取时间代替
}

// SummaryMax 是摘要保留的最大字数（只显示片段，点击跳原文）。
const SummaryMax = 200

// ErrFormat 表示内容不是可识别的 RSS / Atom。
var ErrFormat = errors.New("不是有效的 RSS / Atom 订阅格式")

type textLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Text string `xml:",chardata"`
}

type rssItem struct {
	Title       string     `xml:"title"`
	Links       []textLink `xml:"link"` // RSS 的 <link>文本</link>，也可能夹着 <atom:link href=…/>
	GUID        string     `xml:"guid"`
	PubDate     string     `xml:"pubDate"`
	Date        string     `xml:"date"` // dc:date（RSS 1.0 常用）
	Description string     `xml:"description"`
	Encoded     string     `xml:"encoded"` // content:encoded
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Links     []textLink `xml:"link"`
	ID        string     `xml:"id"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
}

// doc 同时容纳三种格式：<rss><channel>…</channel></rss>、<rdf:RDF><channel/><item/>…、<feed><entry/>…
type doc struct {
	XMLName xml.Name
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	Items   []rssItem   `xml:"item"` // RSS 1.0（RDF）的条目在根元素下
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

// Parse 解析 RSS 2.0 / RSS 1.0（RDF）/ Atom。按 XML 声明自动转码（GBK、GB2312、Big5 等），
// 容忍 &nbsp; 之类的 HTML 实体（很多站点的 RSS 并不是严格合法的 XML）。
func Parse(r io.Reader) (Feed, error) {
	d := xml.NewDecoder(r)
	d.CharsetReader = charset.NewReaderLabel
	d.Strict = false
	d.Entity = xml.HTMLEntity
	var x doc
	if err := d.Decode(&x); err != nil {
		return Feed{}, ErrFormat
	}
	var f Feed
	switch strings.ToLower(x.XMLName.Local) {
	case "rss":
		f.Title = clean(x.Channel.Title, 200)
		for _, it := range x.Channel.Items {
			f.Items = append(f.Items, fromRSS(it))
		}
	case "rdf":
		f.Title = clean(x.Channel.Title, 200)
		for _, it := range x.Items {
			f.Items = append(f.Items, fromRSS(it))
		}
	case "feed":
		f.Title = clean(x.Title, 200)
		for _, e := range x.Entries {
			f.Items = append(f.Items, fromAtom(e))
		}
	default:
		return Feed{}, ErrFormat
	}
	out := f.Items[:0]
	for _, it := range f.Items {
		if it.Title == "" || it.Link == "" { // 没有标题或链接的条目对「看一眼、点开看」没有意义
			continue
		}
		out = append(out, it)
	}
	f.Items = out
	return f, nil
}

func fromRSS(it rssItem) Item {
	link := ""
	for _, l := range it.Links {
		if t := strings.TrimSpace(l.Text); t != "" {
			link = t
			break
		}
	}
	if link == "" {
		for _, l := range it.Links {
			if l.Href != "" && (l.Rel == "" || l.Rel == "alternate") {
				link = strings.TrimSpace(l.Href)
				break
			}
		}
	}
	if link == "" && strings.HasPrefix(strings.TrimSpace(it.GUID), "http") {
		link = strings.TrimSpace(it.GUID)
	}
	desc := it.Description
	if strings.TrimSpace(desc) == "" {
		desc = it.Encoded
	}
	pub := ParseTime(it.PubDate)
	if pub.IsZero() {
		pub = ParseTime(it.Date)
	}
	out := Item{Title: clean(it.Title, 300), Link: link, Summary: clean(desc, SummaryMax), Published: pub}
	out.GUID = guid(strings.TrimSpace(it.GUID), out)
	return out
}

func fromAtom(e atomEntry) Item {
	link := ""
	for _, l := range e.Links {
		if l.Href != "" && (l.Rel == "" || l.Rel == "alternate") {
			link = strings.TrimSpace(l.Href)
			break
		}
	}
	if link == "" && len(e.Links) > 0 {
		link = strings.TrimSpace(e.Links[0].Href)
	}
	sum := e.Summary
	if strings.TrimSpace(sum) == "" {
		sum = e.Content
	}
	pub := ParseTime(e.Published)
	if pub.IsZero() {
		pub = ParseTime(e.Updated)
	}
	out := Item{Title: clean(e.Title, 300), Link: link, Summary: clean(sum, SummaryMax), Published: pub}
	out.GUID = guid(strings.TrimSpace(e.ID), out)
	return out
}

func guid(id string, it Item) string {
	if id != "" {
		return id
	}
	if it.Link != "" {
		return it.Link
	}
	h := sha1.Sum([]byte(it.Title + "\x00" + it.Published.String()))
	return hex.EncodeToString(h[:])
}

var (
	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe = regexp.MustCompile(`[\s\x{00a0}]+`) // 含 &nbsp; 解出来的不换行空格
)

// clean 把可能带 HTML 的文本变成一行纯文本：去标签、解实体、合并空白、截断。
// 摘要里的 HTML 一律不渲染（不执行脚本、不加载外部图片）。
func clean(s string, max int) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(html.UnescapeString(s)) // 有的源会二次转义（&amp;nbsp;）
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max]) + "…"
	}
	return s
}

var timeLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339Nano, time.RFC3339,
	"Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "Mon, 02 Jan 2006 15:04 -0700",
	"2 Jan 2006 15:04:05 -0700", "02 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05-0700", "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05",
	"2006-01-02 15:04:05 -0700", "2006-01-02 15:04:05", "2006/01/02 15:04:05", "2006-01-02",
}

// cst 是中国标准时间。很多国内源的日期不带时区或写成「CST」，按北京时间理解。
var cst = time.FixedZone("CST", 8*3600)

// ParseTime 尽力解析各种常见日期写法；解析不出返回零值。
func ParseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	s = strings.Replace(s, " CST", " +0800", 1) // Go 会把 CST 当成未知时区（偏移 0），这里明确为北京时间
	s = strings.Replace(s, " GMT", " +0000", 1)
	for _, l := range timeLayouts {
		if t, err := time.ParseInLocation(l, s, cst); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ParseBytes 是 Parse 的便捷形式。
func ParseBytes(b []byte) (Feed, error) { return Parse(bytes.NewReader(b)) }
