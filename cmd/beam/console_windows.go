//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func init() {
	// The release binary is a console-subsystem executable, so CMD and
	// PowerShell wait for it and stdout is connected before main runs.
	// Do not call AttachConsole: that joins a parent console and lets a
	// background daemon keep the caller's console after the CLI exits.
	if shouldDetachConsole(os.Args, consoleProcessCount()) {
		hideAndFreeConsole()
	}
}

func consoleProcessCount() int {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel.NewProc("GetConsoleProcessList")
	var processes [32]uint32
	count, _, _ := proc.Call(uintptr(unsafe.Pointer(&processes[0])), uintptr(len(processes)))
	return int(count)
}

func hideAndFreeConsole() {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	user := syscall.NewLazyDLL("user32.dll")
	window, _, _ := kernel.NewProc("GetConsoleWindow").Call()
	if window != 0 {
		const swHide = 0
		user.NewProc("ShowWindow").Call(window, swHide)
	}
	kernel.NewProc("FreeConsole").Call()
}
