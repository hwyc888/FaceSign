//go:build windows

package main

import "syscall"

func hideConsoleIfRequested(args []string) {
	if !backgroundRequested(args) {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		_, _, _ = showWindow.Call(hwnd, 0)
	}
}
