//go:build windows

package firewall

import (
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func platformExec(name string, args []string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func elevate(args []string) error {
	if len(args) == 0 {
		return ErrNeedsElevation
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(args[0])
	if err != nil {
		return err
	}
	params, err := windows.UTF16PtrFromString(strings.Join(args[1:], " "))
	if err != nil {
		return err
	}
	var info shellExecuteInfo
	info.cbSize = uint32(unsafe.Sizeof(info))
	info.fMask = seeMaskNoCloseProcess
	info.lpVerb = uintptr(unsafe.Pointer(verb))
	info.lpFile = uintptr(unsafe.Pointer(file))
	info.lpParameters = uintptr(unsafe.Pointer(params))
	info.nShow = 0
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	r, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return callErr
		}
		return ErrNeedsElevation
	}
	if info.hProcess != 0 {
		handle := windows.Handle(info.hProcess)
		_, _ = windows.WaitForSingleObject(handle, 60*1000)
		_ = windows.CloseHandle(handle)
	}
	return nil
}

const seeMaskNoCloseProcess = 0x00000040

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       uintptr
	lpFile       uintptr
	lpParameters uintptr
	lpDirectory  uintptr
	nShow        int32
	_            uint32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      uintptr
	hkeyClass    uintptr
	dwHotKey     uint32
	_            uint32
	hIcon        uintptr
	hProcess     uintptr
}
