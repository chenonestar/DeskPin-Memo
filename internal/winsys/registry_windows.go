//go:build windows

package winsys

import (
	"errors"
	"os"
	"strings"

	"deskpinmemo/internal/regtrace"

	"golang.org/x/sys/windows/registry"
)

// winReg 是 regtrace.Registry 的真实实现，只操作 HKCU（无需管理员权限）。
type winReg struct{}

func (winReg) GetString(path, name string) (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	return v, err == nil
}

func (winReg) SetString(path, name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (winReg) SetDWord(path, name string, v uint32) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue(name, v)
}

func (winReg) DeleteValue(path, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// DeleteTree 递归删除子项（registry.DeleteKey 只能删除空项）。
func (r winReg) DeleteTree(path string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	subs, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := r.DeleteTree(path + `\` + s); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func (winReg) KeyExists(path string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

func fileExists(p string) bool {
	_, err := os.Stat(strings.TrimSpace(p))
	return err == nil
}

// SyncAutostart 启动时 / 与自启无关的设置被修改时调用：让自启项与设置一致（保守，不动别的有效副本的条目）。
func SyncAutostart(exePath string, enabled bool) (string, error) {
	return regtrace.SyncAutostart(winReg{}, exePath, enabled, fileExists)
}

// RegistryTraces 列出本程序写入 HKCU 的全部项。
func (s *Shell) RegistryTraces() []regtrace.Trace { return regtrace.Traces(winReg{}) }

// ClearRegistry 删除本程序写入 HKCU 的全部项。
func (s *Shell) ClearRegistry() ([]string, error) { return regtrace.Clear(winReg{}) }
