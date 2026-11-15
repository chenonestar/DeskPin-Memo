package feed

// Preset 是预置的订阅源。
type Preset struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Default  bool   `json:"default"` // 首次开启时默认勾选
}

// Presets 只收录国内一般能正常访问、以科技 / 生活为主的源，不收录境外新闻媒体。
// 这些地址在开发环境里无法访问验证，需要在用户的电脑上用「测试连接」确认；打不开的应从这里删除。
var Presets = []Preset{
	{"科技", "少数派", "https://sspai.com/feed", true},
	{"科技", "IT之家", "https://www.ithome.com/rss/", true},
	{"科技", "36氪", "https://36kr.com/feed", false},
	{"科技", "爱范儿", "https://www.ifanr.com/feed", false},
	{"科技", "极客公园", "https://www.geekpark.net/rss", false},
	{"科技", "Solidot", "https://www.solidot.org/index.rss", false},
	{"开发", "开源中国资讯", "https://www.oschina.net/news/rss", false},
	{"开发", "阮一峰的网络日志", "https://www.ruanyifeng.com/blog/atom.xml", false},
	{"综合", "中国新闻网·滚动", "https://www.chinanews.com.cn/rss/scroll-news.xml", true},
	{"软件", "小众软件", "https://www.appinn.com/feed/", false},
	{"游戏", "机核", "https://www.gcores.com/rss", false},
	{"社区", "V2EX", "https://www.v2ex.com/index.xml", false},
}
