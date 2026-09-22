//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func showStartupFailure(startupErr error) {
	logPath := filepath.Join(filepath.Dir(executablePath()), "facesign-error.log")
	if _, err := os.Stat(logPath); err != nil {
		logPath = filepath.Join(os.TempDir(), "facesign-error.log")
	}
	message := fmt.Sprintf(
		"FaceSign 无法启动。\r\n\r\n错误：\r\n%s\r\n\r\n常见原因：\r\n1. 旧版 FaceSign 仍在后台运行，占用了 8080 或 8443 端口；\r\n2. 只复制了 FaceSign.exe，没有一起解压 onnxruntime.dll 和 models；\r\n3. 当前目录没有写入 data/tls 的权限；\r\n4. 轻量包缺少模型且当前网络无法下载模型。\r\n\r\n升级已安装版本，请以管理员身份运行 scripts\\install.ps1。\r\n\r\n错误日志：\r\n%s",
		startupErr.Error(),
		logPath,
	)
	title, _ := syscall.UTF16PtrFromString("FaceSign 启动失败")
	body, _ := syscall.UTF16PtrFromString(message)
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	_, _, _ = messageBox.Call(
		0,
		uintptr(unsafe.Pointer(body)),
		uintptr(unsafe.Pointer(title)),
		0x00000010|0x00040000,
	)
}

func executablePath() string {
	path, err := os.Executable()
	if err != nil {
		return "."
	}
	return path
}
