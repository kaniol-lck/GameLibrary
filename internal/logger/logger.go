// Package logger writes structured application logs.
//
// Log lines are deliberately readable in a plain text editor, because the
// primary consumer is a user attaching a log file to a bug report:
//
//	2026-05-30 14:30:01.234 [INFO] [scanner.go:42] scan started gameDirectories=[.\Games] maxDepth=3
//
// Two operational details matter for a portable NAS application:
//
//   - Logs default to a per-machine local directory. Writing them next to the
//     shared executable would put every client's log on the network share, where
//     they interleave and each write costs a round trip.
//   - Files rotate per day and are named after the date, so logs stay
//     greppable and bounded without a rotation daemon.
//
// Every function is a no-op before Init, which keeps unit tests free of setup.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Options configures the log destination.
type Options struct {
	// LibraryRoot is the directory containing the executable. Logs land in
	// <LibraryRoot>/logs when ToLibrary is set.
	LibraryRoot string
	// LocalDir is a per-machine directory. Logs land in <LocalDir>/logs by
	// default, keeping network traffic off the share.
	LocalDir string
	// ToLibrary forces logging into the shared library folder.
	ToLibrary bool
	// Level is the minimum level that is written.
	Level slog.Level
	// Console additionally mirrors records to stderr, which is what makes
	// `wails dev` usable.
	Console bool
}

// ParseLevel maps a configuration string to a slog level, defaulting to Info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// DefaultLocalDir returns the per-machine directory used when the caller does
// not supply one.
func DefaultLocalDir() string {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "GameLibrary")
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "GameLibrary")
	}
	return ""
}

var (
	stateMu sync.RWMutex
	handler *rotatingHandler
	root    *slog.Logger
)

// Init starts logging. It replaces any previous configuration.
//
// The configured directory is tried first, then the other sensible location, then
// the system temp directory. Logging is the only diagnostic channel a GUI build
// has — a failure to create the log file must not also mean losing the reason the
// application refused to start.
func Init(opts Options) {
	stateMu.Lock()
	defer stateMu.Unlock()

	if handler != nil {
		handler.close()
		handler = nil
		root = nil
	}

	for _, dir := range logDirCandidates(opts) {
		h := &rotatingHandler{dir: dir, level: opts.Level, console: opts.Console}
		if err := h.ensureFile(time.Now()); err == nil {
			handler = h
			root = slog.New(h)
			if opts.Console {
				fmt.Fprintf(os.Stderr, "[logger] writing to %s\n", dir)
			}
			return
		}
	}

	// Last resort: console only, so nothing is written but nothing is lost either.
	handler = &rotatingHandler{dir: "", level: opts.Level, console: true}
	root = slog.New(handler)
}

// logDirCandidates lists the directories to try, most preferred first.
func logDirCandidates(opts Options) []string {
	candidates := make([]string, 0, 4)

	add := func(dir string) {
		if dir == "" {
			return
		}
		for _, existing := range candidates {
			if existing == dir {
				return
			}
		}
		candidates = append(candidates, dir)
	}

	// logDirFor already appends the "logs" segment; the others are bare roots.
	add(logDirFor(opts))
	add(logsUnder(opts.LibraryRoot))
	add(logsUnder(opts.LocalDir))
	add(logsUnder(filepath.Join(os.TempDir(), "GameLibrary")))
	return candidates
}

func logsUnder(base string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(base, "logs")
}

// Reconfigure swaps the destination and level, typically once the shared config
// has been read.
func Reconfigure(opts Options) {
	Init(opts)
}

func logDirFor(opts Options) string {
	base := opts.LocalDir
	if opts.ToLibrary || base == "" {
		base = opts.LibraryRoot
	}
	if base == "" {
		return ""
	}
	return filepath.Join(base, "logs")
}

// Close flushes and closes the active log file.
func Close() {
	stateMu.Lock()
	defer stateMu.Unlock()
	if handler != nil {
		handler.close()
		handler = nil
	}
	// Returning to the zero state means logging after shutdown is a no-op rather
	// than silently reopening the file.
	root = nil
}

// Dir returns the directory logs are currently written to, or "" when logging is
// console-only.
func Dir() string {
	stateMu.RLock()
	defer stateMu.RUnlock()
	if handler == nil {
		return ""
	}
	return handler.dir
}

func Debug(msg string, args ...any) { log(slog.LevelDebug, msg, args...) }
func Info(msg string, args ...any)  { log(slog.LevelInfo, msg, args...) }
func Warn(msg string, args ...any)  { log(slog.LevelWarn, msg, args...) }
func Error(msg string, args ...any) { log(slog.LevelError, msg, args...) }

func log(level slog.Level, msg string, args ...any) {
	stateMu.RLock()
	logger := root
	stateMu.RUnlock()
	if logger == nil {
		return
	}
	// Handle is called directly rather than through Logger.Log, so the level has to
	// be checked here: the handler's Enabled method would otherwise never be
	// consulted and every record would be written regardless of configuration.
	if !logger.Enabled(context.Background(), level) {
		return
	}
	record := slog.NewRecord(time.Now(), level, msg, callerPC())
	record.Add(args...)
	_ = logger.Handler().Handle(context.Background(), record)
}

// callerPC returns the program counter of the first stack frame outside this
// package.
//
// A fixed skip count was wrong for every typed helper (GameLaunched,
// ScrapeSuccess, ...): the constant offset lands on logger.go itself, so the file
// and line printed in the log pointed at the logging wrapper rather than the call
// site. Walking the stack until the frame leaves this package is correct no matter
// how many wrappers are stacked.
func callerPC() uintptr {
	var pcs [16]uintptr
	n := runtime.Callers(2, pcs[:])
	if n == 0 {
		return 0
	}
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if !isLoggerFrame(frame.File) {
			return frame.PC
		}
		if !more {
			return 0
		}
	}
}

func isLoggerFrame(file string) bool {
	slashed := strings.ReplaceAll(file, `\`, "/")
	return strings.Contains(slashed, "/internal/logger/")
}

// rotatingHandler writes one file per day and mirrors to stderr on demand.
type rotatingHandler struct {
	mu      sync.Mutex
	dir     string
	level   slog.Level
	console bool

	file     *os.File
	fileDate string
}

func (h *rotatingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *rotatingHandler) Handle(_ context.Context, r slog.Record) error {
	line := h.format(r)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.console {
		_, _ = os.Stderr.WriteString(line)
	}
	if err := h.ensureFile(r.Time); err != nil {
		return nil // never propagate logging failures into business logic
	}
	_, _ = io.WriteString(h.file, line)
	return nil
}

func (h *rotatingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *rotatingHandler) WithGroup(string) slog.Handler      { return h }

// ensureFile opens today's file, rotating when the date changes.
func (h *rotatingHandler) ensureFile(now time.Time) error {
	if h.dir == "" {
		return fmt.Errorf("no log directory configured")
	}
	date := now.Format("2006-01-02")
	if h.file != nil && h.fileDate == date {
		return nil
	}
	if h.file != nil {
		_ = h.file.Close()
		h.file = nil
	}
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(h.dir, "gamemanager_"+date+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	h.file = file
	h.fileDate = date
	return nil
}

func (h *rotatingHandler) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.file != nil {
		_ = h.file.Close()
		h.file = nil
		h.fileDate = ""
	}
}

func (h *rotatingHandler) format(r slog.Record) string {
	var b strings.Builder
	b.WriteString(r.Time.Format("2006-01-02 15:04:05.000"))
	b.WriteByte(' ')
	b.WriteString(levelString(r.Level))
	b.WriteByte(' ')

	if name, line := source(r.PC); name != "" {
		fmt.Fprintf(&b, "[%s:%d] ", name, line)
	}
	b.WriteString(r.Message)

	r.Attrs(func(a slog.Attr) bool {
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(formatValue(a.Value))
		return true
	})
	b.WriteByte('\n')
	return b.String()
}

// formatValue renders values compactly while keeping slices readable and quoting
// anything that would otherwise be ambiguous.
func formatValue(v slog.Value) string {
	if v.Kind() == slog.KindAny {
		switch typed := v.Any().(type) {
		case []string:
			return "[" + strings.Join(typed, ", ") + "]"
		case error:
			if typed == nil {
				return "<nil>"
			}
			return typed.Error()
		}
	}
	if v.Kind() == slog.KindString {
		s := v.String()
		if strings.ContainsAny(s, " \t") {
			return "\"" + s + "\""
		}
		return s
	}
	return fmt.Sprint(v.Any())
}

func levelString(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "[ERRO]"
	case l >= slog.LevelWarn:
		return "[WARN]"
	case l >= slog.LevelInfo:
		return "[INFO]"
	default:
		return "[DEBG]"
	}
}

func source(pc uintptr) (string, int) {
	if pc == 0 {
		return "", 0
	}
	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	if frame.File == "" {
		return "", 0
	}
	return filepath.Base(frame.File), frame.Line
}

// --- typed helpers ----------------------------------------------------------
//
// These exist so call sites read as intent rather than as a pile of key/value
// pairs. They are thin wrappers; the caller walk in callerPC keeps the reported
// location accurate.

func AppStarted(version, exeDir, hostname string) {
	Info("application started", "version", version, "exeDir", exeDir, "hostname", hostname)
}

func AppStopped() {
	Info("application shutting down")
}

func ConfigLoaded(exeDir string, gameDirs []string, sources int) {
	Info("config loaded", "exeDir", exeDir, "gameDirectories", gameDirs, "sourceCount", sources)
}

func ConfigSaved(exeDir string) {
	Info("config saved", "exeDir", exeDir)
}

func ConfigDefaultCreated(exeDir string) {
	Info("default config created", "exeDir", exeDir)
}

func ConfigMigrated() {
	Info("legacy config fields migrated to metadataSources")
}

func ScanStarted(dirs []string, maxDepth int) {
	Info("scan started", "gameDirectories", dirs, "maxDepth", maxDepth)
}

func ScanFinished(totalDirs, newGames, existingGames int) {
	Info("scan finished", "totalDirs", totalDirs, "newGames", newGames, "existingGames", existingGames)
}

func ScanDirectoryEntered(dir string, depth int) {
	Debug("entering directory", "dir", dir, "depth", depth)
}

func ScanDirectorySkipped(dir string, reason string) {
	Debug("directory skipped", "dir", dir, "reason", reason)
}

func ScanMaxDepthReached(dir string, depth, maxDepth int) {
	Debug("max scan depth reached", "dir", dir, "depth", depth, "maxDepth", maxDepth)
}

func ScanGameAlreadyExists(dir string, id string) {
	Debug("game already scanned, skipping", "dir", dir, "id", id)
}

func ScanGameDiscovered(dir string, id, title, platform string, exeCount int) {
	Info("game discovered", "dir", dir, "id", id, "title", title, "platform", platform, "executables", exeCount)
}

func ScanGameSteamDetected(dir string, appID string) {
	Debug("steam appID detected", "dir", dir, "appId", appID)
}

func ScanGameNoExecutable(dir string) {
	Warn("no launchable file found in game directory", "dir", dir)
}

func ScanGameSaveFailed(dir string, err error) {
	Error("failed to save game info", "dir", dir, "error", err.Error())
}

func ScanExeFound(fileName string) {
	Debug("launcher found", "file", fileName)
}

func ScanExeFiltered(fileName string, reason string) {
	Debug("launcher filtered out", "file", fileName, "reason", reason)
}

func ScanExePrimaryPicked(name string, keyword string) {
	Debug("primary executable selected", "name", name, "keyword", keyword)
}

func ScanExeShortestPicked(name string) {
	Debug("primary executable selected (shortest name)", "name", name)
}

func ScanGameDirNotExist(dir string) {
	Warn("configured game directory does not exist", "dir", dir)
}

func ScrapeStarted(gameID, title string) {
	Info("scrape started", "gameId", gameID, "title", title)
}

func ScrapeSourceSkipped(gameID, sourceKey, reason string) {
	Debug("scrape source skipped", "gameId", gameID, "source", sourceKey, "reason", reason)
}

func ScrapeSourceAttempt(gameID, sourceKey, searchTerm string) {
	Info("scrape source attempt", "gameId", gameID, "source", sourceKey, "searchTerm", searchTerm)
}

func ScrapeSourceFailed(gameID, title, sourceKey string, err error) {
	Warn("scrape source returned error", "gameId", gameID, "title", title, "source", sourceKey, "error", err.Error())
}

func ScrapeSourceEmpty(gameID, title, sourceKey string) {
	Debug("scrape source returned no result", "gameId", gameID, "title", title, "source", sourceKey)
}

func ScrapeSuccess(gameID, title, sourceKey, resultTitle, platformID string) {
	attrs := []any{"gameId", gameID, "title", title, "source", sourceKey, "resultTitle", resultTitle}
	if platformID != "" {
		attrs = append(attrs, "platformId", platformID)
	}
	Info("scrape succeeded", attrs...)
}

func ScrapeAllSourcesFailed(gameID, title string) {
	Warn("scrape failed: no enabled source returned results", "gameId", gameID, "title", title)
}

func ScrapeGameNotFound(id string) {
	Warn("scrape called for unknown game", "gameId", id)
}

func CoverDownloaded(gameID, coverType, url string, err error) {
	if err != nil {
		if url == "" {
			Debug("cover download skipped (source returned no cover)", "gameId", gameID, "type", coverType)
			return
		}
		Warn("cover download failed", "gameId", gameID, "type", coverType, "url", url, "error", err.Error())
		return
	}
	Info("cover downloaded", "gameId", gameID, "type", coverType, "url", url)
}

func GameLaunched(gameID, title, exe string) {
	Info("game launched", "gameId", gameID, "title", title, "exe", exe)
}

func GameLaunchFailed(gameID, title string, err error) {
	Error("game launch failed", "gameId", gameID, "title", title, "error", err.Error())
}

func GameInfoUpdated(id, title string) {
	Debug("game info updated", "gameId", id, "title", title)
}

func GameInfoSaved(id, title string, err error) {
	if err != nil {
		Error("game info save failed", "gameId", id, "title", title, "error", err.Error())
	}
}

func QueueTaskStarted(gameID string, taskType string) {
	Info("queue: task started", "gameId", gameID, "type", taskType)
}

func QueueTaskFinished(gameID string, taskType string, err error, duration time.Duration) {
	if err != nil {
		Warn("queue: task failed", "gameId", gameID, "type", taskType, "error", err.Error(), "duration", duration)
		return
	}
	Info("queue: task completed", "gameId", gameID, "type", taskType, "duration", duration)
}

func WatcherNewDirs(count int) {
	Info("watcher: new game dirs detected", "count", count)
}

func WatcherRemovedDirs(count int) {
	Info("watcher: game dirs removed", "count", count)
}

func WatcherRestarted(enabled bool, directories int) {
	Info("watcher restarted", "enabled", enabled, "directories", directories)
}
