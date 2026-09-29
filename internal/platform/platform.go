// Package platform isolates every operating-system specific operation the
// application performs: opening folders, opening a file in a text editor,
// launching a game, and locating a Steam installation.
//
// GameLibrary targets Windows. Unix-style input paths are still accepted
// everywhere (see internal/fsutil) and a non-Windows build exists only so the
// module keeps compiling and testing on CI; the shell-integration entry points
// return ErrUnsupported there.
package platform

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrUnsupported is returned when an operation has no implementation on the
// current operating system.
var ErrUnsupported = errors.New("operation not supported on this platform")

// LaunchKind describes how a launcher file has to be started.
type LaunchKind int

const (
	// LaunchDirect starts the file as its own process (PE executables, .com).
	LaunchDirect LaunchKind = iota
	// LaunchCommandScript runs the file through the command interpreter (.bat/.cmd).
	LaunchCommandScript
	// LaunchPowerShellScript runs the file through PowerShell (.ps1).
	LaunchPowerShellScript
)

// launcherKinds maps a lower-case extension to the way it must be started.
//
// Games on a NAS library are frequently shipped with a wrapper script next to
// the executable (or instead of one), so scripts are first-class launchers
// rather than being ignored.
var launcherKinds = map[string]LaunchKind{
	".exe":       LaunchDirect,
	".com":       LaunchDirect,
	".bat":       LaunchCommandScript,
	".cmd":       LaunchCommandScript,
	".ps1":       LaunchPowerShellScript,
	".lnk":       LaunchDirect,
	".url":       LaunchDirect,
	".appref-ms": LaunchDirect,
}

// LauncherExtensions returns the recognised launchable extensions, each
// including its leading dot, in lower case.
func LauncherExtensions() []string {
	exts := make([]string, 0, len(launcherKinds))
	for ext := range launcherKinds {
		exts = append(exts, ext)
	}
	return exts
}

// IsLauncher reports whether the file name looks like something the
// application can start.
func IsLauncher(name string) bool {
	_, ok := launcherKinds[strings.ToLower(filepath.Ext(name))]
	return ok
}

// LaunchKindFor returns how name has to be started and whether it is a
// recognised launcher at all.
func LaunchKindFor(name string) (LaunchKind, bool) {
	kind, ok := launcherKinds[strings.ToLower(filepath.Ext(name))]
	return kind, ok
}

// scriptInterpreter builds the argv used to run a script file. It exists so the
// argv shape is unit-testable without executing anything.
func scriptInterpreter(kind LaunchKind, path string) (name string, args []string, ok bool) {
	switch kind {
	case LaunchCommandScript:
		// cmd.exe needs the /c flag; the path is quoted because NAS paths
		// routinely contain spaces.
		return "cmd", []string{"/c", quoteForCmd(path)}, true
	case LaunchPowerShellScript:
		return "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path}, true
	default:
		return "", nil, false
	}
}

func quoteForCmd(s string) string {
	return `"` + s + `"`
}

// ShowFatalError, implemented per platform, reports an unrecoverable startup
// failure to the user.
//
// It exists because the application used to die in total silence: a failure to
// create the WebView2 controller happens before any of the application's own code
// runs, Wails then terminates the process, and the error goes to a stdout stream
// that a GUI build has nowhere to display. Double-clicking the executable simply
// did nothing — no window, no message, no log entry.
//
// Platforms with a GUI show a modal message box; the rest write to stderr so a
// terminal launch still surfaces it.
