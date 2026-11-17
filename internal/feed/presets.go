package feed

// Preset 是预置的订阅源。
type Preset struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Default  bool   `json:"default"` // 首次开启时默认勾选
	// Unverified：地址来自公开资料推断、可信度较低，界面上标「（待验证）」，请用户测试后反馈。
	Unverified bool `json:"unverified"`
}

// Presets 只收录国内一般能正常访问、以科技 / 生活为主的源，不收录境外新闻媒体。
// 这些地址在开发环境里无法访问验证，需要在用户的电脑上用「测试连接」确认；打不开的应从这里删除。
var Presets = []Preset{
	{Category: "科技", Name: "少数派", URL: "https://sspai.com/feed", Default: true},
	{Category: "科技", Name: "IT之家", URL: "https://www.ithome.com/rss/", Default: true},
	{Category: "科技", Name: "36氪", URL: "https://36kr.com/feed"},
	{Category: "科技", Name: "爱范儿", URL: "https://www.ifanr.com/feed"},
	{Category: "科技", Name: "极客公园", URL: "https://www.geekpark.net/rss"},
	{Category: "科技", Name: "Solidot", URL: "https://www.solidot.org/index.rss"},
	{Category: "开发", Name: "开源中国资讯", URL: "https://www.oschina.net/news/rss"},
	{Category: "开发", Name: "阮一峰的网络日志", URL: "https://www.ruanyifeng.com/blog/atom.xml"},
	{Category: "综合", Name: "中国新闻网·滚动", URL: "https://www.chinanews.com.cn/rss/scroll-news.xml", Default: true},
	// 财经（二期）：均默认不勾选
	{Category: "财经", Name: "长桥·金融快讯", URL: "https://longportapp.cn/news/live/feed", Unverified: true},
	{Category: "财经", Name: "FT中文网", URL: "https://www.ftchinese.com/rss/feed", Unverified: true},
	{Category: "财经", Name: "中国新闻网·财经", URL: "https://www.chinanews.com.cn/rss/finance.xml", Unverified: true},
	{Category: "财经", Name: "新浪财经·要闻", URL: "http://rss.sina.com.cn/roll/finance/hot_roll.xml", Unverified: true}, // 老接口，只有 http
	{Category: "软件", Name: "小众软件", URL: "https://www.appinn.com/feed/"},
	{Category: "游戏", Name: "机核", URL: "https://www.gcores.com/rss"},
	{Category: "社区", Name: "V2EX", URL: "https://www.v2ex.com/index.xml"},
}
