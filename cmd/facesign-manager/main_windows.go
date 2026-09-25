//go:build windows

package main

import (
	"context"
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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

const (
	taskName = "FaceSign"

	wmCreate     = 0x0001
	wmDestroy    = 0x0002
	wmCommand    = 0x0111
	wmSetFont         = 0x0030
	wmAsyncDone        = 0x8001
	wmUpgradeSelected  = 0x8002
	wmUpgradeLaunchDone = 0x8003

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
	idUpgrade        = 1011
	idInstall        = 1012
)

var (
	version        = "dev"
	installDirFlag string
	mainWindow     syscall.Handle
	statusBox      syscall.Handle
	actionButtons  = make(map[int]syscall.Handle)
	asyncBusy          bool
	asyncResultMu      sync.Mutex
	asyncResult        *uiActionResult
	upgradeSelectionMu sync.Mutex
	upgradeSelection   *upgradeSelectionResult
	upgradeLaunchMu    sync.Mutex
	upgradeLaunch      *upgradeLaunchResult

	queryTaskStateFn = queryTaskState
	faceSignPIDsFn    = faceSignPIDs
	execHiddenFn      = execHidden
	sleepFn           = time.Sleep

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
	procPostMessageW     = user32.NewProc("PostMessageW")
	procEnableWindow     = user32.NewProc("EnableWindow")

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

type uiActionResult struct {
	Name   string
	Status string
	Err    error
}

type upgradeSelectionResult struct {
	Path string
	Err  error
}

type upgradeLaunchResult struct {
	Err error
}

var (
	errAlreadyRunning        = errors.New("FaceSign 已在运行，无需重复启动")
	errAlreadyStopped        = errors.New("FaceSign 已停止，无需重复停止")
	errStartupAlreadyEnabled = errors.New("FaceSign 开机启动已开启")
	errStartupAlreadyDisabled = errors.New("FaceSign 开机启动已关闭")
)

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
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		return cwd
	}
	return "."
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
	// A Win32 window and its message queue belong to the OS thread that created
	// them. Without pinning this goroutine, Go may resume GetMessageW on a
	// different thread after an async action, which can leave the real window
	// thread without a message pump and make the manager appear "not responding".
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

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
		// WM_CREATE is delivered before CreateWindowExW returns, so publish the
		// real window handle here. The initial async status refresh must post its
		// completion back to this window instead of HWND(0).
		mainWindow = syscall.Handle(hwnd)
		createControls(mainWindow)
		beginAsyncUIAction("读取服务状态", nil)
		return 0
	case wmCommand:
		switch int(wParam & 0xffff) {
		case idStart:
			beginAsyncUIAction("启动 FaceSign", startFaceSign)
		case idStop:
			beginAsyncUIAction("停止 FaceSign", stopFaceSign)
		case idRestart:
			beginAsyncUIAction("重启 FaceSign", restartFaceSign)
		case idEnableStartup:
			beginAsyncUIAction("开启开机启动", enableStartup)
		case idDisableStartup:
			beginAsyncUIAction("关闭开机启动", disableStartup)
		case idOpenWeb:
			beginAsyncUIAction("打开管理网页", openWeb)
		case idOpenLog:
			beginAsyncUIAction("查看启动日志", openLog)
		case idOpenDir:
			beginAsyncUIAction("打开安装目录", func() error { return shellOpen(installDirFlag) })
		case idRefresh:
			beginAsyncUIAction("刷新状态", nil)
		case idUpgrade:
			beginUpgradeSelection()
		case idInstall:
			beginAsyncUIAction("安装/注册本目录", installCurrentDirectory)
		case idExit:
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmAsyncDone:
		finishAsyncUIAction()
		return 0
	case wmUpgradeSelected:
		finishUpgradeSelection(syscall.Handle(hwnd))
		return 0
	case wmUpgradeLaunchDone:
		finishUpgradeLaunch(syscall.Handle(hwnd))
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
	createStatic(hwnd, "便携模式直接运行本目录 FaceSign.exe；需要开机启动时点“安装/注册本目录”，程序和数据不会复制到 C 盘。", 24, 52, 650, 38)

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
		{idOpenDir, "打开当前目录", 24, 402, 150},
		{idUpgrade, "升级 FaceSign", 190, 402, 150},
		{idInstall, "安装/注册本目录", 356, 402, 150},
		{idExit, "关闭管理工具", 522, 402, 150},
	}
	for _, b := range buttons {
		h := createControl("BUTTON", b.text, uintptr(wsChild|wsVisible|wsTabStop|bsPushButton), b.x, b.y, b.w, 34, hwnd, b.id)
		procSendMessageW.Call(uintptr(h), wmSetFont, font, 1)
		if isAsyncActionButton(b.id) {
			actionButtons[b.id] = h
		}
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

func isAsyncActionButton(id int) bool {
	switch id {
	case idStart, idStop, idRestart, idEnableStartup, idDisableStartup, idOpenWeb, idOpenLog, idOpenDir, idRefresh, idUpgrade, idInstall:
		return true
	default:
		return false
	}
}

func setActionButtonsEnabled(enabled bool) {
	value := uintptr(0)
	if enabled {
		value = 1
	}
	for _, button := range actionButtons {
		procEnableWindow.Call(uintptr(button), value)
	}
}

func launchAsync(action func() error, done func(error)) {
	go func() {
		done(action())
	}()
}

func beginAsyncUIAction(name string, action func() error) {
	if asyncBusy {
		return
	}
	asyncBusy = true
	setActionButtonsEnabled(false)
	setStatusText(name + "，请稍候……\r\n\r\n管理窗口仍可正常移动、最小化或关闭。")

	target := mainWindow
	launchAsync(func() error {
		if action == nil {
			return nil
		}
		return action()
	}, func(actionErr error) {
		status, finalErr := finishActionStatus(actionErr)
		result := &uiActionResult{
			Name:   name,
			Status: status,
			Err:    finalErr,
		}
		asyncResultMu.Lock()
		asyncResult = result
		asyncResultMu.Unlock()
		procPostMessageW.Call(uintptr(target), wmAsyncDone, 0, 0)
	})
}

func finishActionStatus(actionErr error) (string, error) {
	switch {
	case errors.Is(actionErr, errAlreadyRunning):
		return errAlreadyRunning.Error() + "。", nil
	case errors.Is(actionErr, errAlreadyStopped):
		return errAlreadyStopped.Error() + "。", nil
	case errors.Is(actionErr, errStartupAlreadyEnabled):
		return errStartupAlreadyEnabled.Error() + "。", nil
	case errors.Is(actionErr, errStartupAlreadyDisabled):
		return errStartupAlreadyDisabled.Error() + "。", nil
	default:
		return buildStatusText(), actionErr
	}
}

func finishAsyncUIAction() {
	asyncResultMu.Lock()
	result := asyncResult
	asyncResult = nil
	asyncResultMu.Unlock()

	asyncBusy = false
	setActionButtonsEnabled(true)
	if result == nil {
		return
	}
	setStatusText(result.Status)
	if result.Err != nil {
		showError(fmt.Errorf("%s失败：%w", result.Name, result.Err))
	}
}

func buildStatusText() string {
	type taskStateResult struct {
		state string
		err   error
	}
	stateCh := make(chan taskStateResult, 1)
	pidsCh := make(chan []int, 1)

	// Task Scheduler and process enumeration are independent external probes.
	// Run them together so a slow probe does not double the time that an action
	// remains in the busy state.
	go func() {
		state, err := queryTaskStateFn()
		stateCh <- taskStateResult{state: state, err: err}
	}()
	go func() {
		pidsCh <- faceSignPIDsFn()
	}()

	info := readStartupInfo()
	certText := rootCertificateStatus()
	stateResult := <-stateCh
	state := stateResult.state
	if stateResult.err != nil {
		state = "查询失败"
	}
	pids := <-pidsCh

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
		taskText = "未安装（便携模式可直接运行）"
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
		"服务状态：%s\r\n计划任务：%s\r\n开机启动：%s\r\nHTTP：%s\r\nHTTPS：%s\r\n运行版本：%s\r\n根证书：%s\r\n当前目录：%s\r\n管理工具版本：%s",
		running, taskText, startupText, httpListen, httpsListen, v, certText, installDirFlag, version,
	)
	return text
}

func queryTaskState() (string, error) {
	script := "[Console]::OutputEncoding=[Text.Encoding]::UTF8; $t=Get-ScheduledTask -TaskName 'FaceSign' -ErrorAction SilentlyContinue; if($null -eq $t){'missing'} else {$t.State.ToString()}"
	out, err := execHiddenFn("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func startFaceSign() error {
	// Repeated Start is intentionally a fast no-op. Re-running an already active
	// scheduled task can block or return an error even though FaceSign is healthy.
	if len(faceSignPIDsFn()) > 0 {
		return errAlreadyRunning
	}

	state, err := queryTaskStateFn()
	if err != nil {
		return fmt.Errorf("读取 FaceSign 计划任务状态失败: %w", err)
	}
	if strings.EqualFold(state, "missing") {
		return errors.New("FaceSign 计划任务尚未安装。便携模式请直接运行本目录 FaceSign.exe；需要开机启动请点击“安装/注册本目录”")
	}
	wasDisabled := strings.EqualFold(state, "disabled")
	if wasDisabled {
		if _, err := execHiddenFn("schtasks.exe", "/Change", "/TN", taskName, "/ENABLE"); err != nil {
			return fmt.Errorf("临时启用计划任务失败: %w", err)
		}
	}
	if _, err := execHiddenFn("schtasks.exe", "/Run", "/TN", taskName); err != nil {
		if wasDisabled {
			_, _ = execHiddenFn("schtasks.exe", "/Change", "/TN", taskName, "/DISABLE")
		}
		return fmt.Errorf("启动 FaceSign 失败: %w", err)
	}
	sleepFn(350 * time.Millisecond)
	if wasDisabled {
		_, _ = execHiddenFn("schtasks.exe", "/Change", "/TN", taskName, "/DISABLE")
	}
	return nil
}

func stopFaceSign() (retErr error) {
	// Repeated Stop is a fast no-op. There is no need to touch Task Scheduler
	// when no FaceSign process exists.
	if len(faceSignPIDsFn()) == 0 {
		return errAlreadyStopped
	}

	state, stateErr := queryTaskStateFn()
	taskExists := stateErr == nil && !strings.EqualFold(state, "missing")
	wasDisabled := strings.EqualFold(state, "disabled")
	temporarilyDisabled := false

	if taskExists && !wasDisabled {
		if _, err := execHiddenFn("schtasks.exe", "/Change", "/TN", taskName, "/DISABLE"); err == nil {
			temporarilyDisabled = true
		}
	}
	if temporarilyDisabled {
		defer func() {
			if _, err := execHiddenFn("schtasks.exe", "/Change", "/TN", taskName, "/ENABLE"); err != nil && retErr == nil {
				retErr = fmt.Errorf("FaceSign 已停止，但恢复开机启动状态失败")
			}
		}()
	}

	if taskExists {
		// First let Task Scheduler terminate the task it owns. Do not immediately
		// fall back to taskkill: the process may need a short moment to disappear.
		_, _ = execHiddenFn("schtasks.exe", "/End", "/TN", taskName)
		if waitForFaceSignExit(8, 125*time.Millisecond) {
			return nil
		}
	}

	// Only the FaceSign parent processes are force-killed. The old /T /IM form
	// also tried to terminate child decoder processes and could return Access
	// Denied even while FaceSign itself was already shutting down. Kill each
	// remaining FaceSign PID independently and judge success by the final process
	// state rather than taskkill's localized exit text.
	remaining := faceSignPIDsFn()
	for _, pid := range remaining {
		_, _ = execHiddenFn("taskkill.exe", "/F", "/PID", strconv.Itoa(pid))
	}
	if waitForFaceSignExit(25, 200*time.Millisecond) {
		return nil
	}

	remaining = faceSignPIDsFn()
	values := make([]string, len(remaining))
	for i, pid := range remaining {
		values[i] = strconv.Itoa(pid)
	}
	return fmt.Errorf(
		"FaceSign 仍有进程未退出（PID: %s）。计划任务已停止，但 Windows 未能结束这些残留进程；请检查安全软件或系统进程保护",
		strings.Join(values, ", "),
	)
}

func waitForFaceSignExit(maxChecks int, interval time.Duration) bool {
	if maxChecks < 1 {
		maxChecks = 1
	}
	for i := 0; i < maxChecks; i++ {
		if len(faceSignPIDsFn()) == 0 {
			return true
		}
		if i+1 < maxChecks {
			sleepFn(interval)
		}
	}
	return false
}

func restartFaceSign() error {
	if len(faceSignPIDsFn()) > 0 {
		if err := stopFaceSign(); err != nil && !errors.Is(err, errAlreadyStopped) {
			return err
		}
	}
	if err := startFaceSign(); err != nil {
		return fmt.Errorf("重启 FaceSign 失败: %w", err)
	}
	return nil
}

func enableStartup() error {
	state, err := queryTaskStateFn()
	if err != nil {
		return fmt.Errorf("读取 FaceSign 计划任务状态失败: %w", err)
	}
	if strings.EqualFold(state, "missing") {
		return errors.New("FaceSign 计划任务尚未安装")
	}
	if !strings.EqualFold(state, "disabled") {
		return errStartupAlreadyEnabled
	}
	if _, err := execHiddenFn("schtasks.exe", "/Change", "/TN", taskName, "/ENABLE"); err != nil {
		return fmt.Errorf("开启开机启动失败: %w", err)
	}
	return nil
}

func disableStartup() error {
	state, err := queryTaskStateFn()
	if err != nil {
		return fmt.Errorf("读取 FaceSign 计划任务状态失败: %w", err)
	}
	if strings.EqualFold(state, "missing") {
		return errors.New("FaceSign 计划任务尚未安装")
	}
	if strings.EqualFold(state, "disabled") {
		return errStartupAlreadyDisabled
	}
	if _, err := execHiddenFn("schtasks.exe", "/Change", "/TN", taskName, "/DISABLE"); err != nil {
		return fmt.Errorf("关闭开机启动失败: %w", err)
	}
	return nil
}


func installPowerShellCommand(installDir string) string {
	script := filepath.Join(installDir, "scripts", "install.ps1")
	return fmt.Sprintf(
		"& %s -InstallDir %s -OpenBrowser:$false",
		powerShellLiteral(script),
		powerShellLiteral(installDir),
	)
}

func installCurrentDirectory() error {
	required := []string{
		filepath.Join(installDirFlag, "FaceSign.exe"),
		filepath.Join(installDirFlag, "FaceSignManager.exe"),
		filepath.Join(installDirFlag, "onnxruntime.dll"),
		filepath.Join(installDirFlag, "scripts", "install.ps1"),
	}
	for _, path := range required {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return fmt.Errorf("当前目录不是完整的 FaceSign 发布包，缺少：%s", filepath.Base(path))
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command", installPowerShellCommand(installDirFlag),
	)
	cmd.Dir = installDirFlag
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("安装超过10分钟，已自动终止；请检查网络、模型文件或 Windows 计划任务")
	}
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" || !utf8.ValidString(message) {
			message = err.Error()
		}
		return fmt.Errorf("安装本目录失败: %s", message)
	}
	return nil
}

func beginUpgradeSelection() {
	if asyncBusy {
		return
	}
	asyncBusy = true
	setActionButtonsEnabled(false)
	setStatusText("请选择解压后的 FaceSign 新版本目录。\r\n\r\n目录选择期间管理窗口仍可正常移动、最小化或关闭。")

	target := mainWindow
	go func() {
		path, err := chooseUpgradePackage()
		if err == nil && strings.TrimSpace(path) != "" {
			path = normalizeUpgradePackageDir(path)
			err = validateUpgradePackage(path)
		}
		upgradeSelectionMu.Lock()
		upgradeSelection = &upgradeSelectionResult{Path: path, Err: err}
		upgradeSelectionMu.Unlock()
		procPostMessageW.Call(uintptr(target), wmUpgradeSelected, 0, 0)
	}()
}

func finishUpgradeSelection(hwnd syscall.Handle) {
	upgradeSelectionMu.Lock()
	result := upgradeSelection
	upgradeSelection = nil
	upgradeSelectionMu.Unlock()

	asyncBusy = false
	setActionButtonsEnabled(true)
	if result == nil {
		return
	}
	if result.Err != nil {
		setStatusText("选择升级目录失败。")
		showError(result.Err)
		return
	}
	if strings.TrimSpace(result.Path) == "" {
		setStatusText("已取消升级。")
		return
	}

	packageDir := result.Path
	message := "将使用这个新版本目录升级 FaceSign：\r\n\r\n" + packageDir +
		"\r\n\r\n升级时管理工具会自动关闭，升级完成后自动重新打开。" +
		"\r\n数据库、人脸数据、证书、端口和开机启动状态都会保留。\r\n\r\n是否继续？"
	if !confirmBox(mainWindow, message, "升级 FaceSign") {
		setStatusText("已取消升级。")
		return
	}
	beginUpgradeLaunch(packageDir)
}

func beginUpgradeLaunch(packageDir string) {
	asyncBusy = true
	setActionButtonsEnabled(false)
	setStatusText("正在启动 FaceSign 升级程序……\r\n\r\n管理窗口仍可正常移动、最小化或关闭。")

	target := mainWindow
	go func() {
		err := launchUpgradeHelper(packageDir)
		upgradeLaunchMu.Lock()
		upgradeLaunch = &upgradeLaunchResult{Err: err}
		upgradeLaunchMu.Unlock()
		procPostMessageW.Call(uintptr(target), wmUpgradeLaunchDone, 0, 0)
	}()
}

func finishUpgradeLaunch(hwnd syscall.Handle) {
	upgradeLaunchMu.Lock()
	result := upgradeLaunch
	upgradeLaunch = nil
	upgradeLaunchMu.Unlock()

	asyncBusy = false
	if result == nil {
		setActionButtonsEnabled(true)
		return
	}
	if result.Err != nil {
		setActionButtonsEnabled(true)
		setStatusText("启动升级程序失败。")
		showError(result.Err)
		return
	}

	setStatusText("升级程序已启动。\r\n\r\n管理工具即将关闭；升级完成后会自动重新打开。")
	procDestroyWindow.Call(uintptr(hwnd))
}

func chooseUpgradePackage() (string, error) {
	script := fmt.Sprintf(
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8; $s=New-Object -ComObject Shell.Application; $f=$s.BrowseForFolder(%d,'请选择解压后的 FaceSign 新版本目录',0x41,0); if($null -ne $f){$f.Self.Path}",
		uintptr(mainWindow),
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("打开升级目录选择窗口失败: %s", message)
	}
	return strings.TrimSpace(string(out)), nil
}

func normalizeUpgradePackageDir(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if strings.EqualFold(filepath.Base(clean), "scripts") {
		if _, err := os.Stat(filepath.Join(clean, "upgrade.ps1")); err == nil {
			parent := filepath.Dir(clean)
			if _, err := os.Stat(filepath.Join(parent, "FaceSign.exe")); err == nil {
				return parent
			}
		}
	}
	return clean
}

func validateUpgradePackage(packageDir string) error {
	if packageDir == "" || packageDir == "." {
		return errors.New("没有选择有效的新版本目录")
	}
	packageAbs, err := filepath.Abs(packageDir)
	if err != nil {
		return err
	}
	installAbs, err := filepath.Abs(installDirFlag)
	if err == nil && strings.EqualFold(filepath.Clean(packageAbs), filepath.Clean(installAbs)) {
		return errors.New("请选择新下载并解压的 FaceSign 发布目录，不能选择当前安装目录")
	}

	required := []string{
		"FaceSign.exe",
		"FaceSignManager.exe",
		"onnxruntime.dll",
		filepath.Join("scripts", "install.ps1"),
		filepath.Join("scripts", "upgrade.ps1"),
	}
	for _, relative := range required {
		path := filepath.Join(packageDir, relative)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return fmt.Errorf("所选目录不是完整的 FaceSign 新版本包，缺少：%s", relative)
		}
	}
	return nil
}

func launchUpgradeHelper(packageDir string) error {
	upgradeScript := filepath.Join(packageDir, "scripts", "upgrade.ps1")
	managerPath := filepath.Join(installDirFlag, "FaceSignManager.exe")
	command := fmt.Sprintf(
		"$ErrorActionPreference='Stop'; $Host.UI.RawUI.WindowTitle='FaceSign 升级'; "+
			"Write-Host '等待管理工具退出后开始升级...'; Wait-Process -Id %d -ErrorAction SilentlyContinue; "+
			"try { & %s -InstallDir %s -OpenBrowser:$false; "+
			"Write-Host ''; Write-Host 'FaceSign 升级完成，正在重新打开管理工具...'; "+
			"$managerArgs='--install-dir \"' + %s + '\"'; Start-Process -FilePath %s -ArgumentList $managerArgs; Start-Sleep -Seconds 2 } "+
			"catch { Write-Host ''; Write-Host ('FaceSign 升级失败：' + $_.Exception.Message) -ForegroundColor Red; "+
			"Write-Host ''; Write-Host '按回车键关闭此窗口。'; [void][Console]::ReadLine(); exit 1 }",
		os.Getpid(),
		powerShellLiteral(upgradeScript),
		powerShellLiteral(installDirFlag),
		powerShellLiteral(installDirFlag),
		powerShellLiteral(managerPath),
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
	cmd.Dir = packageDir
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法启动升级程序: %w", err)
	}
	return nil
}

func powerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func confirmBox(parent syscall.Handle, text, title string) bool {
	r, _, _ := procMessageBoxW.Call(
		uintptr(parent),
		uintptr(unsafe.Pointer(utf16(text))),
		uintptr(unsafe.Pointer(utf16(title))),
		0x00000004|0x00000020,
	)
	return r == 6
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
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("%s 执行超过8秒，已自动终止", filepath.Base(name))
	}
	if err != nil {
		return out, errors.New(commandErrorMessage(name, out, err))
	}
	return out, nil
}

func commandErrorMessage(name string, out []byte, err error) string {
	message := strings.TrimSpace(string(out))
	if message == "" || !utf8.ValidString(message) {
		return fmt.Sprintf("%s 执行失败: %v", filepath.Base(name), err)
	}
	return message
}

func faceSignPIDs() []int {
	out, err := execHiddenFn("tasklist.exe", "/FI", "IMAGENAME eq FaceSign.exe", "/FO", "CSV", "/NH")
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
