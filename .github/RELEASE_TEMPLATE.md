## Installation

1. Download `GameLibrary.exe`
2. Place it in your NAS game library root directory, alongside your `Games/` folder
3. Run `GameLibrary.exe` from any machine with the NAS mounted

```
NAS:\GameLibrary\
├── GameLibrary.exe          ← put here
├── config.json              ← auto-generated on first run
└── Games\                   ← your game directories
    ├── GameA\
    │   └── .gameinfo.json
    └── GameB\
```

## Usage

1. Mount NAS to any drive letter on each client machine
2. Run `GameLibrary.exe`
3. Click **Scan Games** to discover your library
4. Click **Scrape All** to download metadata and covers
5. Click any game card to view details

## Verification

```
SHA256: see checksum.txt in release assets
```
