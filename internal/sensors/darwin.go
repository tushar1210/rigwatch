//go:build darwin

package sensors

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/tushar1210/rigwatch/internal/metrics"
)

// readPlatform reads the Apple SMC through IOKit. No sudo, no cgo: purego
// calls the system frameworks directly so a CGO_ENABLED=0 binary still works.
func readPlatform(_ context.Context, _ string) Reading {
	temps, fans, err := readSMC()
	if err != nil {
		return Reading{Hint: "SMC unavailable"}
	}
	if len(temps) == 0 && len(fans) == 0 {
		return Reading{Hint: "SMC returned no temperature or fan keys"}
	}
	return Reading{Temps: temps, Fans: fans}
}

type smcConn struct {
	call    func(conn uint32, selector uint32, in unsafe.Pointer, inSize uintptr, out unsafe.Pointer, outSize *uintptr) int32
	closeFn func(conn uint32) int32
	release func(object uint32) int32
	conn    uint32
	keys    []string
	keysAt  time.Time
	mu      sync.Mutex
	openErr error
}

var (
	smcOnce sync.Once
	smcBox  *smcConn
)

func readSMC() ([]metrics.Temp, []metrics.Fan, error) {
	smcOnce.Do(func() {
		smcBox = openSMC()
	})
	if smcBox.openErr != nil {
		return nil, nil, smcBox.openErr
	}
	return smcBox.sample()
}

func openSMC() *smcConn {
	c := &smcConn{}
	iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		c.openErr = err
		return c
	}
	libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		c.openErr = err
		return c
	}
	var (
		matching func(name string) uintptr
		get      func(port uint32, dict uintptr) uint32
		open     func(service uint32, task uint32, typ uint32, conn *uint32) int32
	)
	if err := bind(iokit, "IOServiceMatching", &matching); err != nil {
		c.openErr = err
		return c
	}
	if err := bind(iokit, "IOServiceGetMatchingService", &get); err != nil {
		c.openErr = err
		return c
	}
	if err := bind(iokit, "IOServiceOpen", &open); err != nil {
		c.openErr = err
		return c
	}
	if err := bind(iokit, "IOConnectCallStructMethod", &c.call); err != nil {
		c.openErr = err
		return c
	}
	if err := bind(iokit, "IOServiceClose", &c.closeFn); err != nil {
		c.openErr = err
		return c
	}
	if err := bind(iokit, "IOObjectRelease", &c.release); err != nil {
		c.openErr = err
		return c
	}
	task, err := machTaskSelf(libSystem)
	if err != nil {
		c.openErr = err
		return c
	}
	dict := matching("AppleSMC")
	if dict == 0 {
		c.openErr = errSMC("AppleSMC matching dictionary was empty")
		return c
	}
	service := get(0, dict)
	if service == 0 {
		c.openErr = errSMC("AppleSMC service not found")
		return c
	}
	var conn uint32
	if ret := open(service, task, 0, &conn); ret != 0 || conn == 0 {
		c.release(service)
		c.openErr = errSMC("IOServiceOpen failed")
		return c
	}
	c.release(service)
	c.conn = conn
	return c
}

type errSMC string

func (e errSMC) Error() string { return string(e) }

func bind(lib uintptr, name string, fn any) error {
	sym, err := purego.Dlsym(lib, name)
	if err != nil {
		return err
	}
	purego.RegisterFunc(fn, sym)
	return nil
}

func machTaskSelf(libSystem uintptr) (uint32, error) {
	sym, err := purego.Dlsym(libSystem, "mach_task_self_")
	if err == nil && sym != 0 {
		return uint32(readU32(sym)), nil
	}
	var fn func() uint32
	if err := bind(libSystem, "mach_task_self", &fn); err != nil {
		return 0, err
	}
	return fn(), nil
}

func (c *smcConn) sample() ([]metrics.Temp, []metrics.Fan, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.keysAt) > time.Minute || len(c.keys) == 0 {
		c.keys = c.enumerate()
		c.keysAt = time.Now()
	}
	var temps []metrics.Temp
	var fans []metrics.Fan
	for _, key := range c.keys {
		raw, typ, ok := c.readKey(key)
		if !ok {
			continue
		}
		val, typeName, decoded := decodeSMC(typ, raw)
		if !decoded {
			continue
		}
		switch {
		case isSMCTemp(key, typeName):
			if val < -20 || val > 150 {
				continue
			}
			temps = append(temps, metrics.Temp{
				Name:    smcTempName(key),
				Celsius: val,
				Source:  "smc",
				Kind:    smcKind(key),
			})
		case isSMCFan(key, typeName):
			fans = append(fans, metrics.Fan{
				Name:   key,
				RPM:    val,
				HasRPM: true,
				Source: "smc",
			})
		}
	}
	return temps, fans, nil
}

func (c *smcConn) enumerate() []string {
	countRaw, typ, ok := c.readKey("#KEY")
	if !ok {
		return fallbackSMCKeys()
	}
	count, _, decoded := decodeSMC(typ, countRaw)
	if !decoded || count <= 0 || count > 8000 {
		return fallbackSMCKeys()
	}
	var keys []string
	for i := 0; i < int(count); i++ {
		in := smcKeyData{Data8: smcCmdReadIndex, Data32: uint32(i)}
		out, ok := c.callSMC(in)
		if !ok {
			continue
		}
		key := fourCCString(out.Key)
		if len(key) != 4 {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return fallbackSMCKeys()
	}
	return keys
}

func (c *smcConn) readKey(key string) ([]byte, uint32, bool) {
	infoIn := smcKeyData{Key: fourCC(key), Data8: smcCmdReadInfo}
	info, ok := c.callSMC(infoIn)
	if !ok || info.KeyInfo.DataSize == 0 || info.KeyInfo.DataSize > 32 {
		return nil, 0, false
	}
	readIn := smcKeyData{Key: fourCC(key), Data8: smcCmdReadBytes}
	readIn.KeyInfo.DataSize = info.KeyInfo.DataSize
	out, ok := c.callSMC(readIn)
	if !ok {
		return nil, 0, false
	}
	n := info.KeyInfo.DataSize
	buf := make([]byte, n)
	copy(buf, out.Bytes[:n])
	return buf, info.KeyInfo.DataType, true
}

func (c *smcConn) callSMC(in smcKeyData) (smcKeyData, bool) {
	var out smcKeyData
	outSize := uintptr(unsafe.Sizeof(out))
	ret := c.call(c.conn, smcKernelIndex, unsafe.Pointer(&in), uintptr(unsafe.Sizeof(in)), unsafe.Pointer(&out), &outSize)
	return out, ret == 0
}
