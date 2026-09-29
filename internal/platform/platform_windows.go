//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// OpenPath reveals path in the OS shell. Directories open in Explorer and files
// are handed to their default association.
func OpenPath(path string) error {
	if strings.TrimSpace(path) == "" {
		// Checked before cleaning: filepath.Clean("") is ".", which would
		// silently open the process working directory instead of failing.
		return fmt.Errorf("open path: empty path")
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open path %s: %w", path, err)
	}
	return exec.Command("explorer", path).Start()
}

// EditFile opens path in Notepad.
func EditFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("edit file: empty path")
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("edit file %s: %w", path, err)
	}
	return exec.Command("notepad", path).Start()
}

// LaunchGame starts a game launcher file and returns the started process.
//
// Scripts are routed through their interpreter because Windows cannot start a
// .bat/.ps1 through CreateProcess directly. The working directory is always the
// game directory so relative asset paths resolve the way the game expects.
func LaunchGame(path, workDir string) (*exec.Cmd, error) {
	kind, ok := LaunchKindFor(path)
	if !ok {
		return nil, fmt.Errorf("unsupported launcher type: %s", filepath.Base(path))
	}

	var cmd *exec.Cmd
	if name, args, isScript := scriptInterpreter(kind, path); isScript {
		cmd = exec.Command(name, args...)
	} else {
		cmd = exec.Command(path)
	}
	cmd.Dir = workDir
	// Detach so a console window does not linger for GUI launches.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(path), err)
	}
	return cmd, nil
}

// SteamUser is a detected Steam account.
type SteamUser struct {
	ID   string
	Name string
}

// SteamPath returns the Steam installation directory, or "" when Steam cannot
// be found.
func SteamPath() string {
	if path, err := registryReadString(`HKEY_CURRENT_USER\Software\Valve\Steam`, "SteamPath"); err == nil {
		if path = filepath.Clean(path); dirExists(path) {
			return path
		}
	}
	if path, err := registryReadString(`HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"); err == nil {
		if path = filepath.Clean(path); dirExists(path) {
			return path
		}
	}
	// Only probe fixed drives: walking every mapped network drive here would
	// stall startup on a client with many NAS mappings.
	for _, drive := range []string{`C:`, `D:`, `E:`, `F:`, `G:`} {
		for _, suffix := range []string{`\Program Files (x86)\Steam`, `\Steam`, `\Games\Steam`} {
			candidate := drive + suffix
			if dirExists(candidate) {
				return candidate
			}
		}
	}
	return ""
}

// SteamUsers lists the Steam accounts found under userdata/, resolving each
// display name from localconfig.vdf. Accounts whose name cannot be resolved are
// still returned so the user can pick them.
func SteamUsers() []SteamUser {
	steamPath := SteamPath()
	if steamPath == "" {
		return nil
	}
	userdataDir := filepath.Join(steamPath, "userdata")
	entries, err := os.ReadDir(userdataDir)
	if err != nil {
		return nil
	}

	users := make([]SteamUser, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !isNumeric(e.Name()) {
			continue
		}
		users = append(users, SteamUser{
			ID:   e.Name(),
			Name: readPersonaName(filepath.Join(userdataDir, e.Name(), "config", "localconfig.vdf")),
		})
	}
	return users
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// readPersonaName extracts the first "PersonaName" value from a VDF file. The
// file is a nested key/value dump; the top-level occurrence is the account's
// display name.
func readPersonaName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)
	key := `"PersonaName"`
	idx := strings.Index(content, key)
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(key):]
	start := strings.Index(rest, `"`)
	if start < 0 {
		return ""
	}
	rest = rest[start+1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func registryReadString(keyPath, valueName string) (string, error) {
	parts := strings.SplitN(keyPath, `\`, 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid registry key path: %s", keyPath)
	}

	var base uintptr
	switch strings.ToUpper(parts[0]) {
	case "HKEY_CURRENT_USER":
		base = 0x80000001
	case "HKEY_LOCAL_MACHINE":
		base = 0x80000002
	default:
		return "", fmt.Errorf("unsupported registry hive: %s", parts[0])
	}

	subKey, err := syscall.UTF16PtrFromString(parts[1])
	if err != nil {
		return "", err
	}
	value, err := syscall.UTF16PtrFromString(valueName)
	if err != nil {
		return "", err
	}

	const keyRead = 0x20019
	var handle syscall.Handle
	ret, _, _ := procRegOpenKeyEx.Call(base, uintptr(unsafe.Pointer(subKey)), 0, keyRead, uintptr(unsafe.Pointer(&handle)))
	if ret != 0 {
		return "", fmt.Errorf("RegOpenKeyEx(%s) failed: %d", keyPath, ret)
	}
	defer procRegCloseKey.Call(uintptr(handle))

	var buf [1024]uint16
	bufLen := uint32(len(buf)) * 2
	var valueType uint32
	ret, _, _ = procRegQueryValueEx.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(value)),
		0,
		uintptr(unsafe.Pointer(&valueType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufLen)),
	)
	if ret != 0 {
		return "", fmt.Errorf("RegQueryValueEx(%s\\%s) failed: %d", keyPath, valueName, ret)
	}
	return strings.TrimRight(syscall.UTF16ToString(buf[:bufLen/2]), "\x00"), nil
}

var (
	advapi32            = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyEx    = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueEx = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey     = advapi32.NewProc("RegCloseKey")

	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

// ShowFatalError displays a modal message box. It is intentionally the last thing
// the process does, so it blocks until the user acknowledges it.
func ShowFatalError(title, message string) {
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	messagePtr, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	const (
		mbOK            = 0x00000000
		mbIconError     = 0x00000010
		mbSetForeground = 0x00010000
	)
	_, _, _ = procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOK|mbIconError|mbSetForeground),
	)
}
