// +build !windows

package gou

import (
	"syscall"
	"unsafe"
)

// _TIOCGWINSZ is used to check for a terminal under linux
const (
	_TIOCGWINSZ = 0x5413 // OSX 1074295912
)

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// Determine is this process is running in a Terminal or not?
func IsTerminal() bool {
	ws := &winsize{}
	isTerm := true
	defer func() {
		if r := recover(); r != nil {
			isTerm = false
		}
	}()
	// This blows up on windows
	retCode, _, _ := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(_TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))

	if int(retCode) == -1 {
		return false
	}
	return isTerm
}
