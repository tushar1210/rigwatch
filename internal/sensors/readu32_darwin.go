//go:build darwin

package sensors

// readU32 reads a 32-bit value at a C symbol address returned by dlsym.
func readU32(addr uintptr) uintptr
