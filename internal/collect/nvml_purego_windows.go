//go:build windows

package collect

import (
	"sync"
	"syscall"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

var (
	dynOnce sync.Once
	dynLibP *dynLib
	dynErr  error
)

func readNVML() ([]metrics.GPU, error) {
	dynOnce.Do(func() {
		dynLibP, dynErr = openWindowsNVML()
	})
	if dynErr != nil {
		return nil, dynErr
	}
	return dynLibP.gpus()
}

func openWindowsNVML() (*dynLib, error) {
	handle, err := syscall.LoadLibrary("nvml.dll")
	if err != nil {
		return nil, err
	}
	return loadDyn(func(name string) (uintptr, error) {
		return syscall.GetProcAddress(handle, name)
	})
}
