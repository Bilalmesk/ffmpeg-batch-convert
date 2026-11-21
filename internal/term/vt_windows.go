//go:build windows

package term

import (
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

// enableVirtualTerminal switches the Windows console into ANSI mode. Windows
// Terminal already supports it; this also covers classic conhost.
func enableVirtualTerminal() {
	handle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || handle == 0 {
		return
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return
	}
	mode |= enableVirtualTerminalProcessing
	_, _, _ = procSetConsoleMode.Call(uintptr(handle), uintptr(mode))
	// Make Unicode filenames render correctly in classic conhost too.
	_, _, _ = procSetConsoleOutputCP.Call(uintptr(65001))
}
