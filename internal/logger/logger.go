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

var (
	mu     sync.Mutex
	logDir string
	file   *os.File
	today  string
	root   *slog.Logger
)

func Init(dir string) {
	logDir = filepath.Join(dir, "logs")
	os.MkdirAll(logDir, 0755)
	today = dateStr()
	root = slog.New(&dailyHandler{})
}

func dateStr() string {
	return time.Now().Format("2006-01-02")
}

func logFilePath(day string) string {
	return filepath.Join(logDir, fmt.Sprintf("gamemanager_%s.log", day))
}

func Debug(msg string, args ...any) { log(slog.LevelDebug, msg, args...) }
func Info(msg string, args ...any)  { log(slog.LevelInfo, msg, args...) }
func Warn(msg string, args ...any)  { log(slog.LevelWarn, msg, args...) }
func Error(msg string, args ...any) { log(slog.LevelError, msg, args...) }

func log(level slog.Level, msg string, args ...any) {
	if root == nil {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = root.Handler().Handle(context.Background(), r)
}

type dailyHandler struct {
	mu sync.Mutex
}

func (h *dailyHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *dailyHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	day := dateStr()
	if day != today || file == nil {
		if file != nil {
			file.Close()
		}
		today = day
		var err error
		file, err = os.OpenFile(logFilePath(day), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
	}

	w := file

	ts := r.Time.Format("2006-01-02 15:04:05.000")
	level := levelStr(r.Level)

	fn, line := source(r.PC)
	src := fmt.Sprintf("[%s:%d]", fn, line)

	var b strings.Builder
	b.WriteString(ts)
	b.WriteString(" ")
	b.WriteString(level)
	b.WriteString(" ")
	b.WriteString(src)
	b.WriteString(" ")
	b.WriteString(r.Message)

	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(" ")
		b.WriteString(a.Key)
		b.WriteString("=")
		b.WriteString(fmt.Sprint(a.Value.Any()))
		return true
	})
	b.WriteString("\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func (h *dailyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *dailyHandler) WithGroup(name string) slog.Handler {
	return h
}

func levelStr(l slog.Level) string {
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
		return "unknown", 0
	}
	fs := runtime.CallersFrames([]uintptr{pc})
	f, _ := fs.Next()
	file := filepath.Base(f.File)
	return file, f.Line
}

func Close() {
	if file != nil {
		file.Close()
		file = nil
	}
}

func AppStarted(version, exeDir, hostname string) {
	Info("application started",
		"version", version,
		"exeDir", exeDir,
		"hostname", hostname,
	)
}

func ConfigLoaded(exeDir string, gameDirs []string, sources int) {
	Info("config loaded",
		"exeDir", exeDir,
		"gameDirectories", gameDirs,
		"sourceCount", sources,
	)
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
	Info("scan started",
		"gameDirectories", dirs,
		"maxDepth", maxDepth,
	)
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
	Info("game discovered",
		"dir", dir,
		"id", id,
		"title", title,
		"platform", platform,
		"executables", exeCount,
	)
}

func ScanGameSteamDetected(dir string, appID string) {
	Debug("steam appID detected", "dir", dir, "appId", appID)
}

func ScanGameNoExecutable(dir string) {
	Warn("no executable found in game directory", "dir", dir)
}

func ScanGameSaveFailed(dir string, err error) {
	Error("failed to save game info", "dir", dir, "error", err.Error())
}

func ScanExeFound(fileName string) {
	Debug("executable found", "file", fileName)
}

func ScanExeFiltered(fileName string, reason string) {
	Debug("executable filtered out", "file", fileName, "reason", reason)
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

func ScrapeSourceAttempt(gameID, sourceKey string) {
	Debug("scrape source attempt", "gameId", gameID, "source", sourceKey)
}

func ScrapeSourceFailed(gameID, title, sourceKey string, err error) {
	Warn("scrape source returned error",
		"gameId", gameID,
		"title", title,
		"source", sourceKey,
		"error", err.Error(),
	)
}

func ScrapeSourceEmpty(gameID, title, sourceKey string) {
	Debug("scrape source returned no result",
		"gameId", gameID,
		"title", title,
		"source", sourceKey,
	)
}

func ScrapeSuccess(gameID, title, sourceKey, resultTitle, platformID string) {
	attrs := []any{
		"gameId", gameID,
		"title", title,
		"source", sourceKey,
		"resultTitle", resultTitle,
	}
	if platformID != "" {
		attrs = append(attrs, "platformId", platformID)
	}
	Info("scrape succeeded", attrs...)
}

func ScrapeAllSourcesFailed(gameID, title string) {
	Warn("scrape failed: no enabled source returned results",
		"gameId", gameID,
		"title", title,
	)
}

func ScrapeGameNotFound(id string) {
	Warn("scrape called for unknown game", "gameId", id)
}

func CoverDownloaded(gameID, coverType, url string, err error) {
	if err != nil {
		Warn("cover download failed",
			"gameId", gameID,
			"type", coverType,
			"url", url,
			"error", err.Error(),
		)
	} else {
		Info("cover downloaded",
			"gameId", gameID,
			"type", coverType,
			"url", url,
		)
	}
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
