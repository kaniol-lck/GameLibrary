//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func getSteamPath() string {
	path, err := registryReadString(`HKEY_CURRENT_USER\Software\Valve\Steam`, "SteamPath")
	if err == nil && path != "" {
		return filepath.Clean(path)
	}

	defaults := []string{
		`C:\Program Files (x86)\Steam`,
		`D:\Steam`,
		`E:\Steam`,
	}
	for _, d := range defaults {
		if _, err := os.Stat(d); err == nil {
			return d
		}
	}
	return ""
}

func registryReadString(keyPath, valueName string) (string, error) {
	parts := strings.SplitN(keyPath, `\`, 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid key path")
	}
	var base syscall.Handle
	switch strings.ToUpper(parts[0]) {
	case "HKEY_CURRENT_USER":
		base = 0x80000001
	case "HKEY_LOCAL_MACHINE":
		base = 0x80000002
	default:
		return "", fmt.Errorf("unsupported hive: %s", parts[0])
	}

	subKey, _ := syscall.UTF16PtrFromString(parts[1])
	valName, _ := syscall.UTF16PtrFromString(valueName)

	var h syscall.Handle
	r0, _, _ := procRegOpenKeyEx.Call(uintptr(base), uintptr(unsafe.Pointer(subKey)), 0, 0x20019, uintptr(unsafe.Pointer(&h)))
	if r0 != 0 {
		return "", fmt.Errorf("reg open failed: %d", r0)
	}
	defer procRegCloseKey.Call(uintptr(h))

	var buf [512]uint16
	var bufLen uint32 = uint32(len(buf)) * 2
	var typ uint32
	r0, _, _ = procRegQueryValueEx.Call(uintptr(h), uintptr(unsafe.Pointer(valName)), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bufLen)))
	if r0 != 0 {
		return "", fmt.Errorf("reg query failed: %d", r0)
	}
	return syscall.UTF16ToString(buf[:bufLen/2]), nil
}

var (
	advapi32            = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyEx    = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueEx = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey     = advapi32.NewProc("RegCloseKey")
)
