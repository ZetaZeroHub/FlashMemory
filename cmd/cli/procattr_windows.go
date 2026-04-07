//go:build windows

package cli

import "syscall"

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// CREATE_NO_WINDOW = 0x08000000
		CreationFlags: 0x08000000,
	}
}
