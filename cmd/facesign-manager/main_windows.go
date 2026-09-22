//go:build windows

package main

import (
	"crypto/x509"
	"encoding/csv"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	taskName = "FaceSign"

	wmCreate  = 0x0001
	wmDestroy = 0x0002
	wmCommand = 0x0111
	wmSetFont = 0x0030

	wsVisible      = 0x10000000
	wsChild        = 0x40000000
	wsBorder       = 0x00800000
	wsVScroll      = 0x00200000
	wsTabStop      = 0x00010000
	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsMinimizeBox  = 0x00020000
	esMultiline    = 0x0004
	esReadonly     = 0x0800
	esAutoVScroll  = 0x0040
	bsPushButton   = 0x00000000
	colorBtnFace   = 15
	defaultGUIFont = 17
	swShowNormal   = 1

	idStart          = 1001
	idStop           = 1002
	idRestart        = 1003
	idEnableStartup  = 1004
	idDisableStartup = 1005
	idOpenWeb        = 1006
	idOpenLog        = 1007
	idOpenDir        = 1008
	idRefresh        = 1009
	idExit           = 1010
)

var (
	version        = "dev"
	installDirFlag string
	mainWindow     syscall.Handle
	statusBox      syscall.Handle

	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procSetWindowTextW   = user32.NewProc("SetWindowTextW")
	procSendMessageW     = user32.NewProc("SendMessageW")
	procMessageBoxW      = user32.NewProc("MessageBoxW")
	procLoadCursorW      = user32.NewProc("LoadCursorW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procDestroyWindow    = user32.NewProc("DestroyWindow")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procGetStockObject   = gdi32.NewProc("GetStockObject")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
	procIsUserAnAdmin    = shell32.NewProc("IsUserAnAdmin")
)

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type startupInfo struct {
	Version     string
	HTTPListen  string
	HTTPSListen string
	URL         string
	RootCA      string
}

func main() {
	flag.StringVar(&installDirFlag, "install-dir", "", "FaceSign installation directory")
	flag.Parse()

	if !isAdministrator() {
		if err := relaunchElevated(); err != nil {
			messageBox(0, "FaceSign 管理工具需要管理员权限。\r\n\r\n"+err.Error(), "FaceSign 管理工具", 0x10)
		}
		return
	}
	if installDirFlag == "" {
		installDirFlag = defaultInstallDir()
	}
	if err := runGUI(); err != nil {
		messageBox(0, err.Error(), "FaceSign 管理工具", 0x10)
	}
}

func defaultInstallDir() string {
	if programData := strings.TrimSpace(os.Getenv("ProgramData")); programData != "" {
		candidate := filepath.Join(programData, "FaceSign")
		if _, err := os.Stat(filepath.Join(candidate, "FaceSign.exe")); err == nil {
			return candidate
		}
	}
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	return filepath.Join("C:\\ProgramData", "FaceSign")
}

func isAdministrator() bool {
	r, _, _ := procIsUserAnAdmin.Call()
	return r != 0
}

func relaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb := utf16("runas")
	file := utf16(exe)
	args := ""
	if len(os.Args) > 1 {
		quoted := make([]string, 0, len(os.Args)-1)
		for _, arg := range os.Args[1:] {
			quoted = append(quoted, strconv.Quote(arg))
		}
		args = strings.Join(quoted, " ")
	}
	params := utf16(args)
	dir := utf16(filepath.Dir(exe))
	r, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		uintptr(unsafe.Pointer(dir)),
		swShowNormal,
	)
	if r <= 32 {
		return fmt.Errorf("无法请求管理员权限: %v", callErr)
	}
	return nil
}

func runGUI() error {
	instance, _, _ := procGetModuleHandleW.Call(0)
	className := utf16("FaceSignManagerWindow")
	cursor, _, _ := procLoadCursorW.Call(0, 32512)
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   syscall.NewCallback(windowProc),
		HInstance:     syscall.Handle(instance),
		HCursor:       syscall.Handle(cursor),
		HbrBackground: syscall.Handle(colorBtnFace + 1),
		LpszClassName: className,
	}
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("注册管理窗口失败: %v", err)
	}

	width, height := 720, 510
	screenW, _, _ := procGetSystemMetrics.Call(0)
	screenH, _, _ := procGetSystemMetrics.Call(1)
	x := (int(screenW) - width) / 2
	y := (int(screenH) - height) / 2
	style := uintptr(wsCaption | wsSysMenu | wsMinimizeBox)
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16("FaceSign 管理工具"))),
		style,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("创建管理窗口失败: %v", err)
	}
	mainWindow = syscall.Handle(hwnd)
	procShowWindow.Call(hwnd, 5)
	procUpdateWindow.Call(hwnd)

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmCreate:
		createControls(syscall.Handle(hwnd))
		refreshStatus()
		return 0
	case wmCommand:
		switch int(wParam & 0xffff) {
		case idStart:
			runUIAction("启动 FaceSign", startFaceSign)
		case idStop:
			runUIAction("停止 FaceSign", stopFaceSign)
		case idRestart:
			runUIAction("重启 FaceSign", restartFaceSign)
		case idEnableStartup:
			runUIAction("开启开机启动", enableStartup)
		case idDisableStartup:
			runUIAction("关闭开机启动", disableStartup)
		case idOpenWeb:
			if err := openWeb(); err != nil {
				showError(err)
			}
		case idOpenLog:
			if err := openLog(); err != nil {
				showError(err)
			}
		case idOpenDir:
			if err := shellOpen(installDirFlag); err != nil {
				showError(err)
			}
		case idRefresh:
			refreshStatus()
		case idExit:
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func createControls(hwnd syscall.Handle) {
	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	createStatic(hwnd, "FaceSign 服务管理", 24, 20, 650, 28)
	createStatic(hwnd, "用于管理以 SYSTEM 权限运行的 FaceSign。停止服务时会先结束计划任务，再终止残留进程。", 24, 52, 650, 38)

	statusBox = createControl("EDIT", "", uintptr(wsChild|wsVisible|wsBorder|wsVScroll|esMultiline|esReadonly|esAutoVScroll), 24, 96, 650, 190, hwnd, 0)
	procSendMessageW.Call(uintptr(statusBox), wmSetFont, font, 1)

	buttons := []struct {
		id         int
		text       string
		x, y, w    int
	}{
		{idStart, "启动 FaceSign", 24, 310, 150},
		{idStop, "停止 FaceSign", 190, 310, 150},
		{idRestart, "重启 FaceSign", 356, 310, 150},
		{idRefresh, "刷新状态", 522, 310, 150},
		{idEnableStartup, "开启开机启动", 24, 356, 150},
		{idDisableStartup, "关闭开机启动", 190, 356, 150},
		{idOpenWeb, "打开管理网页", 356, 356, 150},
		{idOpenLog, "查看启动日志", 522, 356, 150},
		{idOpenDir, "打开安装目录", 24, 402, 150},
		{idExit, "关闭管理工具", 522, 402, 150},
	}
	for _, b := range buttons {
		h := createControl("BUTTON", b.text, uintptr(wsChild|wsVisible|wsTabStop|bsPushButton), b.x, b.y, b.w, 34, hwnd, b.id)
		procSendMessageW.Call(uintptr(h), wmSetFont, font, 1)
	}
}

func createStatic(parent syscall.Handle, text string, x, y, w, h int) syscall.Handle {
	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	control := createControl("STATIC", text, uintptr(wsChild|wsVisible), x, y, w, h, parent, 0)
	procSendMessageW.Call(uintptr(control), wmSetFont, font, 1)
	return control
}

func createControl(class, text string, style uintptr, x, y, w, h int, parent syscall.Handle, id int) syscall.Handle {
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16(class))),
		uintptr(unsafe.Pointer(utf16(text))),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent), uintptr(id), 0, 0,
	)
	return syscall.Handle(hwnd)
}

func runUIAction(name string, action func() error) {
	setStatusText(name + "，请稍候……")
	if err := action(); err != nil {
		showError(err)
	}
	refreshStatus()
}

func refreshStatus() {
	state, err := queryTaskState()
	if err != nil {
		state = "查询失败"
	}
	pids := faceSignPIDs()
	info := readStartupInfo()
	certText := rootCertificateStatus()

	running := "未运行"
	if len(pids) > 0 {
		values := make([]string, len(pids))
		for i, pid := range pids {
			values[i] = strconv.Itoa(pid)
		}
		running = "运行中，PID: " + strings.Join(values, ", ")
	}
	taskText := "未安装"
	startupText := "-"
	switch strings.ToLower(state) {
	case "running", "ready", "queued":
		taskText = "已安装"
		startupText = "已启用"
	case "disabled":
		taskText = "已安装"
		startupText = "已关闭"
	case "missing":
		taskText = "未安装"
		startupText = "-"
	case "查询失败":
		taskText = "查询失败"
		startupText = "查询失败"
	default:
		if state != "" {
			taskText = "已安装 (" + state + ")"
			startupText = "已启用"
		}
	}

	httpListen := info.HTTPListen
	if httpListen == "" {
		httpListen = "默认 0.0.0.0:8080"
	}
	httpsListen := info.HTTPSListen
	if httpsListen == "" {
		httpsListen = "默认 0.0.0.0:8443"
	}
	v := info.Version
	if v == "" {
		v = "未读取"
	}
	text := fmt.Sprintf(
		"服务状态：%s\r\n计划任务：%s\r\n开机启动：%s\r\nHTTP：%s\r\nHTTPS：%s\r\n运行版本：%s\r\n根证书：%s\r\n安装目录：%s\r\n管理工具版本：%s",
		running, taskText, startupText, httpListen, httpsListen, v, certText, installDirFlag, version,
	)
	setStatusText(text)
}

func queryTaskState() (string, error) {
	script := "[Console]::OutputEncoding=[Text.Encoding]::UTF8; $t=Get-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue; if($null -eq $t){'missing'} else {$t.State.ToString()}"
	out, err := execHidden("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func startFaceSign() error {
	state, _ := queryTaskState()
	if strings.EqualFold(state, "missing") {
		return errors.New("FaceSign 计划任务尚未安装。请先以管理员身份运行发布包中的 scripts\\install.ps1")
	}
	wasDisabled := strings.EqualFold(state, "disabled")
	if wasDisabled {
		if _, err := execHidden("schtasks.exe", "/Change", "/TN", taskName, "/ENABLE"); err != nil {
			return fmt.Errorf("临时启用计划任务失败: %w", err)
		}
	}
	if _, err := execHidden("schtasks.exe", "/Run", "/TN", taskName); err != nil {
		if wasDisabled {
			_, _ = execHidden("schtasks.exe", "/Change", "/TN", taskName, "/DISABLE")
		}
		return fmt.Errorf("启动 FaceSign 失败: %w", err)
	}
	time.Sleep(800 * time.Millisecond)
	if wasDisabled {
		_, _ = execHidden("schtasks.exe", "/Change", "/TN", taskName, "/DISABLE")
	}
	return nil
}

func stopFaceSign() error {
	_, _ = execHidden("schtasks.exe", "/End", "/TN", taskName)
	time.Sleep(300 * time.Millisecond)
	if len(faceSignPIDs()) > 0 {
		_, _ = execHidden("taskkill.exe", "/F", "/T", "/IM", "FaceSign.exe")
	}
	for i := 0; i < 10; i++ {
		if len(faceSignPIDs()) == 0 {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("FaceSign 进程仍未退出。请检查是否有安全软件拦截，或查看 Windows 事件日志")
}

func restartFaceSign() error {
	state, _ := queryTaskState()
	wasDisabled := strings.EqualFold(state, "disabled")
	if err := stopFaceSign(); err != nil {
		return err
	}
	if wasDisabled {
		if _, err := execHidden("schtasks.exe", "/Change", "/TN", taskName, "/ENABLE"); err != nil {
			return err
		}
	}
	if _, err := execHidden("schtasks.exe", "/Run", "/TN", taskName); err != nil {
		return fmt.Errorf("重启 FaceSign 失败: %w", err)
	}
	time.Sleep(800 * time.Millisecond)
	if wasDisabled {
		_, _ = execHidden("schtasks.exe", "/Change", "/TN", taskName, "/DISABLE")
	}
	return nil
}

func enableStartup() error {
	if _, err := execHidden("schtasks.exe", "/Change", "/TN", taskName, "/ENABLE"); err != nil {
		return fmt.Errorf("开启开机启动失败: %w", err)
	}
	return nil
}

func disableStartup() error {
	if _, err := execHidden("schtasks.exe", "/Change", "/TN", taskName, "/DISABLE"); err != nil {
		return fmt.Errorf("关闭开机启动失败: %w", err)
	}
	return nil
}

func openWeb() error {
	info := readStartupInfo()
	url := strings.TrimSpace(info.URL)
	if url == "" {
		if info.HTTPSListen != "" && info.HTTPSListen != "disabled" {
			if _, port, err := net.SplitHostPort(info.HTTPSListen); err == nil {
				url = "https://127.0.0.1:" + port + "/"
			}
		}
	}
	if url == "" {
		url = "https://127.0.0.1:8443/"
	}
	return shellOpen(url)
}

func openLog() error {
	startup := filepath.Join(installDirFlag, "data", "facesign-startup.log")
	errorLog := filepath.Join(installDirFlag, "facesign-error.log")
	path := startup
	if _, err := os.Stat(path); err != nil {
		if _, err2 := os.Stat(errorLog); err2 == nil {
			path = errorLog
		} else {
			return errors.New("暂时没有找到 FaceSign 启动日志")
		}
	}
	cmd := exec.Command("notepad.exe", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func shellOpen(target string) error {
	verb := utf16("open")
	file := utf16(target)
	r, _, err := procShellExecuteW.Call(
		uintptr(mainWindow),
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0, 0, swShowNormal,
	)
	if r <= 32 {
		return fmt.Errorf("无法打开 %s: %v", target, err)
	}
	return nil
}

func execHidden(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return out, errors.New(message)
	}
	return out, nil
}

func faceSignPIDs() []int {
	out, err := execHidden("tasklist.exe", "/FI", "IMAGENAME eq FaceSign.exe", "/FO", "CSV", "/NH")
	if err != nil || !strings.Contains(strings.ToLower(string(out)), "facesign.exe") {
		return nil
	}
	reader := csv.NewReader(strings.NewReader(string(out)))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil
	}
	var pids []int
	for _, row := range rows {
		if len(row) < 2 || !strings.EqualFold(strings.TrimSpace(row[0]), "FaceSign.exe") {
			continue
		}
		pid, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(row[1]), ",", ""))
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

func readStartupInfo() startupInfo {
	data, err := os.ReadFile(filepath.Join(installDirFlag, "data", "facesign-startup.log"))
	if err != nil {
		return startupInfo{}
	}
	values := parseKeyValueLog(string(data))
	return startupInfo{
		Version:     values["version"],
		HTTPListen:  values["http_listen"],
		HTTPSListen: values["https_listen"],
		URL:         values["url"],
		RootCA:      values["root_ca"],
	}
}

func parseKeyValueLog(text string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}

func rootCertificateStatus() string {
	path := filepath.Join(installDirFlag, "tls", "facesign-root-ca.crt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "未找到"
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "证书格式错误"
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "证书解析失败"
	}
	if time.Now().After(cert.NotAfter) {
		return "已过期 " + cert.NotAfter.Format("2006-01-02")
	}
	return "正常，有效至 " + cert.NotAfter.Format("2006-01-02")
}

func setStatusText(text string) {
	if statusBox == 0 {
		return
	}
	procSetWindowTextW.Call(uintptr(statusBox), uintptr(unsafe.Pointer(utf16(text))))
}

func showError(err error) {
	messageBox(mainWindow, err.Error(), "FaceSign 管理工具", 0x10)
}

func messageBox(parent syscall.Handle, text, title string, flags uintptr) {
	procMessageBoxW.Call(
		uintptr(parent),
		uintptr(unsafe.Pointer(utf16(text))),
		uintptr(unsafe.Pointer(utf16(title))),
		flags,
	)
}

func utf16(value string) *uint16 {
	ptr, _ := syscall.UTF16PtrFromString(value)
	return ptr
}
