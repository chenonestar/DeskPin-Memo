// Package hlc 实现混合逻辑时钟（HLC），V1 用于维护 hlc / field_hlc，V3 同步直接复用。
//
// HLC 字符串格式（定长、可按字典序排序）：
//
//	13 位毫秒时间戳 + 4 位逻辑计数（十六进制）+ 设备 id 前 8 位
package hlc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tsWidth      = 13
	counterWidth = 4
	nodeWidth    = 8
	// Len 是 HLC 字符串的固定长度。
	Len = tsWidth + counterWidth + nodeWidth
)

// Clock 是线程安全的本地 HLC 时钟。
type Clock struct {
	mu      sync.Mutex
	node    string
	lastMs  int64
	counter uint32
	now     func() time.Time
}

// New 创建时钟。deviceID 取前 8 位（不足则补 0，去掉连字符）。
func New(deviceID string) *Clock {
	return &Clock{node: nodeID(deviceID), now: time.Now}
}

// NewWithNow 用于测试注入时间源。
func NewWithNow(deviceID string, now func() time.Time) *Clock {
	return &Clock{node: nodeID(deviceID), now: now}
}

func nodeID(deviceID string) string {
	s := strings.ReplaceAll(deviceID, "-", "")
	if len(s) > nodeWidth {
		s = s[:nodeWidth]
	}
	return s + strings.Repeat("0", nodeWidth-len(s))
}

// Next 生成新的 HLC，保证严格单调递增（即使系统时间被回拨）。
func (c *Clock) Next() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ms := c.now().UnixMilli()
	if ms > c.lastMs {
		c.lastMs = ms
		c.counter = 0
	} else {
		c.counter++
		if c.counter > 0xFFFF {
			// 计数溢出：推进毫秒位
			c.lastMs++
			c.counter = 0
		}
	}
	return format(c.lastMs, c.counter, c.node)
}

// Observe 接收远端 HLC，使本地时钟不落后于它（V3 同步拉取时使用）。
func (c *Clock) Observe(remote string) {
	ms, ctr, _, err := Parse(remote)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ms > c.lastMs || (ms == c.lastMs && ctr > c.counter) {
		c.lastMs, c.counter = ms, ctr
	}
}

// Seed 用数据库中已有的最大 HLC 初始化，避免重启后时间回拨导致倒退。
func (c *Clock) Seed(maxHLC string) { c.Observe(maxHLC) }

func format(ms int64, ctr uint32, node string) string {
	return fmt.Sprintf("%013d%04x%s", ms, ctr, node)
}

// Parse 拆解 HLC 字符串。
func Parse(s string) (ms int64, counter uint32, node string, err error) {
	if len(s) != Len {
		return 0, 0, "", fmt.Errorf("hlc: bad length %d", len(s))
	}
	ms, err = strconv.ParseInt(s[:tsWidth], 10, 64)
	if err != nil {
		return
	}
	c, err := strconv.ParseUint(s[tsWidth:tsWidth+counterWidth], 16, 32)
	if err != nil {
		return
	}
	return ms, uint32(c), s[tsWidth+counterWidth:], nil
}

// Compare 比较两个 HLC：-1 / 0 / 1。定长字符串，字典序即全序。
func Compare(a, b string) int { return strings.Compare(a, b) }

// FieldHLC 是 field_hlc 列的 JSON 对象：字段名 → HLC。
type FieldHLC map[string]string

// NewFieldHLC 新建记录：所有字段写入同一个 HLC。
func NewFieldHLC(h string, fields ...string) FieldHLC {
	f := make(FieldHLC, len(fields))
	for _, k := range fields {
		f[k] = h
	}
	return f
}

// Touch 只更新被修改字段的 HLC。
func (f FieldHLC) Touch(h string, fields ...string) FieldHLC {
	if f == nil {
		f = FieldHLC{}
	}
	for _, k := range fields {
		f[k] = h
	}
	return f
}

// JSON 序列化（键有序，输出稳定）。
func (f FieldHLC) JSON() string {
	if f == nil {
		return "{}"
	}
	b, _ := json.Marshal(map[string]string(f))
	return string(b)
}

// ParseFieldHLC 从 JSON 反序列化。
func ParseFieldHLC(s string) FieldHLC {
	f := FieldHLC{}
	if s == "" {
		return f
	}
	_ = json.Unmarshal([]byte(s), &f)
	return f
}

// Merge 按字段合并：每个字段取 HLC 更大者的来源。返回 true 表示 remote 胜出。
func Merge(local, remote FieldHLC) (winners map[string]bool) {
	winners = map[string]bool{}
	for k, rv := range remote {
		if Compare(rv, local[k]) > 0 {
			winners[k] = true
		}
	}
	return
}
