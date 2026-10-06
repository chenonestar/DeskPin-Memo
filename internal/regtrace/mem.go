package regtrace

import (
	"strconv"
	"strings"
	"sync"
)

// MemRegistry 是内存实现：单元测试和开发服务器（非 Windows）使用。
type MemRegistry struct {
	mu   sync.Mutex
	vals map[string]string // key: 路径小写 + "\x00" + 值名小写
	keys map[string]bool
}

// NewMemRegistry 创建空的内存注册表。
func NewMemRegistry() *MemRegistry {
	return &MemRegistry{vals: map[string]string{}, keys: map[string]bool{}}
}

func norm(p string) string { return strings.ToLower(strings.Trim(p, `\`)) }

func (m *MemRegistry) touch(path string) {
	p := norm(path)
	for {
		m.keys[p] = true
		i := strings.LastIndex(p, `\`)
		if i < 0 {
			return
		}
		p = p[:i]
	}
}

func (m *MemRegistry) GetString(path, name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vals[norm(path)+"\x00"+strings.ToLower(name)]
	return v, ok
}

func (m *MemRegistry) SetString(path, name, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.touch(path)
	m.vals[norm(path)+"\x00"+strings.ToLower(name)] = value
	return nil
}

func (m *MemRegistry) SetDWord(path, name string, v uint32) error {
	return m.SetString(path, name, strconv.FormatUint(uint64(v), 10))
}

func (m *MemRegistry) DeleteValue(path, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vals, norm(path)+"\x00"+strings.ToLower(name))
	return nil
}

func (m *MemRegistry) DeleteTree(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := norm(path)
	for k := range m.keys {
		if k == p || strings.HasPrefix(k, p+`\`) {
			delete(m.keys, k)
		}
	}
	for k := range m.vals {
		kp := k[:strings.Index(k, "\x00")]
		if kp == p || strings.HasPrefix(kp, p+`\`) {
			delete(m.vals, k)
		}
	}
	return nil
}

func (m *MemRegistry) KeyExists(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keys[norm(path)]
}
