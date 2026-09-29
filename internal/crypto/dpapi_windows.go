//go:build windows

package crypto

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPIProtect 使用 CryptProtectData（当前用户范围）保护数据。
func DPAPIProtect(data []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(data)), Data: unsafe.SliceData(data)}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

// DPAPIUnprotect 使用 CryptUnprotectData 解密。
func DPAPIUnprotect(data []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(data)), Data: unsafe.SliceData(data)}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

// DPAPIAvailable 表示当前平台支持 DPAPI。
const DPAPIAvailable = true
