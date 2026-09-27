//go:build windows

package main

import (
	"os"
	"syscall"
)

const (
	stdOutputHandle = ^uintptr(10) // -11
	stdErrorHandle  = ^uintptr(11) // -12
)

func init() {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	result, _, _ := kernel.NewProc("AttachConsole").Call(^uintptr(0))
	if result == 0 {
		return
	}
	getHandle := kernel.NewProc("GetStdHandle")
	if file := standardHandle(getHandle, stdOutputHandle, "stdout"); file != nil {
		os.Stdout = file
	}
	if file := standardHandle(getHandle, stdErrorHandle, "stderr"); file != nil {
		os.Stderr = file
	}
}

func standardHandle(get *syscall.LazyProc, kind uintptr, name string) *os.File {
	handle, _, _ := get.Call(kind)
	if handle == 0 || handle == ^uintptr(0) {
		return nil
	}
	return os.NewFile(handle, name)
}
