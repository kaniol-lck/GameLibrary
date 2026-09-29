## Installation

1. Download `GameLibrary-windows-amd64.exe`
2. Place it in your NAS game library root directory, alongside your `Games/` folder
3. Run it from any machine with the NAS mounted

```
NAS:\GameLibrary\
├── GameLibrary-windows-amd64.exe   ← put here (rename to GameLibrary.exe if you like)
├── config.json                     ← auto-generated on first run
└── Games\                          ← your game directories
    ├── GameA\
    │   ├── .gamemanager\           ← metadata lives here, not in the game root
    │   │   └── gameinfo.json
    │   └── game.exe
    └── GameB\
```

## Usage

1. Mount the NAS to any drive letter on each client machine
2. Run the executable
3. Click **Scan** to discover your library (or **Force Scan** to re-identify
   everything — stars, tags and scraped metadata are preserved)
4. Click **Scrape Missing** to fetch metadata for games that still lack it, or
   **Scrape All** to refresh everything
5. Click any game card to open its details

### Metadata scraping

- Scraping runs several games at once (default 4, adjustable in Settings →
  Scanning). Each metadata service keeps its own rate limit, so raising this
  overlaps waiting rather than increasing the request rate any one service sees.
- The sidebar task list shows real progress, every game being scraped right now,
  what is waiting, and recent failures with their reason. Failed entries have a
  retry button.
- Pause, resume, clear the waiting list, or dismiss the finished results.

### If the window does not appear

The window is created by the Microsoft Edge WebView2 runtime. If it cannot start,
the application writes a log and explains itself:

1. Run `run-with-console.cmd` (included in the release) to see the error directly.
2. Check the log in `%LOCALAPPDATA%\GameLibrary\logs`.
3. If the WebView2 runtime is missing or damaged, install or repair it from
   <https://developer.microsoft.com/microsoft-edge/webview2/>.

## Verification

```
SHA256: see GameLibrary-windows-amd64.exe.sha256 in the release assets
```

Verify on Windows with:

```powershell
Get-FileHash .\GameLibrary-windows-amd64.exe -Algorithm SHA256
```
