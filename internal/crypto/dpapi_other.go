//go:build !windows

package crypto

import "errors"

var errNoDPAPI = errors.New("DPAPI 仅在 Windows 可用")

func DPAPIProtect([]byte) ([]byte, error)   { return nil, errNoDPAPI }
func DPAPIUnprotect([]byte) ([]byte, error) { return nil, errNoDPAPI }

const DPAPIAvailable = false
