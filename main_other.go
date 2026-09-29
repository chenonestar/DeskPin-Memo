//go:build !windows

// DeskPin Memo 只在 Windows 上运行。非 Windows 平台仅用于开发：
// 业务层可在任意平台测试，界面可用 `go run ./cmd/devserver` 在浏览器中预览。
package main

import "fmt"

func main() {
	fmt.Println("DeskPin Memo 仅支持 Windows 10/11（x64）。开发预览请运行：go run ./cmd/devserver")
}
