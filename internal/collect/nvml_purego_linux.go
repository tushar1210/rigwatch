//go:build linux && !cgo

package collect

import (
	"sync"

	"github.com/ebitengine/purego"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

var (
	dynOnce sync.Once
	dynLibP *dynLib
	dynErr  error
)

func readNVML() ([]metrics.GPU, error) {
	dynOnce.Do(func() {
		dynLibP, dynErr = openLinuxNVML()
	})
	if dynErr != nil {
		return nil, dynErr
	}
	return dynLibP.gpus()
}

func openLinuxNVML() (*dynLib, error) {
	var handle uintptr
	var err error
	for _, name := range []string{"libnvidia-ml.so.1", "libnvidia-ml.so"} {
		handle, err = purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return loadDyn(func(name string) (uintptr, error) {
		return purego.Dlsym(handle, name)
	})
}
