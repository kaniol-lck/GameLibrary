package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"GameLibrary/internal/logger"
	"GameLibrary/internal/platform"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Logging is initialised here, before the webview exists, and not only in
	// OnStartup. Everything that can go wrong while creating the WebView2
	// environment and its controller happens before OnStartup runs, and the
	// process used to disappear with no log entry and no message — double-clicking
	// the executable simply did nothing. An operator can now always find out what
	// happened, either in the log file or in the dialog below.
	logger.Init(logger.Options{
		LocalDir: logger.DefaultLocalDir(),
		Level:    logger.ParseLevel("info"),
		Console:  true,
	})

	app := NewApp()
	logger.Info("starting application",
		"version", version,
		"exeDir", app.exeDir,
		"logDir", logger.Dir(),
	)

	// A previous run that never produced a window is reported before this one
	// tries to start, because the failure itself happens before any code of ours
	// can run.
	checkPreviousStartup()
	markStartupInProgress()

	err := wails.Run(&options.App{
		Title:     "GameLibrary",
		Width:     1280,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Cover art is served over HTTP rather than through the binding layer.
			// Requests the bundled assets cannot answer fall through to this
			// handler in both development and production builds.
			Handler: app.assetHandler(),
		},
		BackgroundColour: &options.RGBA{R: 15, G: 15, B: 25, A: 1},
		OnStartup:        app.startup,
		OnShutdown: func(ctx context.Context) {
			app.shutdown()
		},
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			// Customised so a missing or broken WebView2 runtime produces a
			// message a user can act on rather than a silent exit.
			Messages: fatalMessages(),
		},
	})

	if err != nil {
		reportStartupFailure(err, app.exeDir)
	}
}

// fatalMessages overrides Wails' built-in WebView2 dialogs with text that says
// what to do about it.
func fatalMessages() *windows.Messages {
	messages := windows.DefaultMessages()
	messages.Webview2NotInstalled = "GameLibrary needs the Microsoft Edge WebView2 runtime, which is not installed.\n\n" +
		"Install it from https://developer.microsoft.com/microsoft-edge/webview2/ and start GameLibrary again."
	messages.InstallationRequired = "GameLibrary needs the Microsoft Edge WebView2 runtime.\n\n" +
		"Install it from https://developer.microsoft.com/microsoft-edge/webview2/ and start GameLibrary again."
	messages.UpdateRequired = "The installed Microsoft Edge WebView2 runtime is too old for GameLibrary.\n\n" +
		"Update it from https://developer.microsoft.com/microsoft-edge/webview2/ and start GameLibrary again."
	messages.WebView2ProcessCrash = "The GameLibrary window stopped responding and has to close.\n\n" +
		"Start GameLibrary again. If it keeps happening, see the log file for details."
	return messages
}

// reportStartupFailure surfaces a fatal startup error both in the log and in a
// dialog, then exits.
func reportStartupFailure(err error, exeDir string) {
	logDir := logger.Dir()
	logger.Error("application failed to start", "error", err.Error())
	logger.Close()

	message := fmt.Sprintf(
		"GameLibrary could not start.\n\n%v\n\nProgram folder:\n%s\n\nLog folder:\n%s",
		err,
		exeDir,
		filepath.Clean(logDir),
	)
	platform.ShowFatalError("GameLibrary failed to start", message)
	os.Exit(1)
}
