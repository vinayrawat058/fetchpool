# Concurrent Downloader

A small Go CLI that downloads multiple files from the internet **at the same time**.

Instead of fetching links one after another, it uses a worker pool so several downloads can run in parallel. You control how many run at once with the `-workers` flag. Finished files are saved to a local folder (by default `downloads/`).

## What this project does

- Accepts a list of HTTP/HTTPS URLs
- Downloads them concurrently (configurable worker count)
- Saves each file under an output directory
- Avoids overwriting existing files by appending `-1`, `-2`, etc.
- **Duplicate detection** — skips the same URL again, and skips saving when content SHA-256 already exists
- **Live progress** — shows active downloads with % (when known), bytes, and speed
- Prints success / skip / failure for each URL, plus totals at the end

### Project layout

```
cmd/downloader/          CLI entrypoint
internal/downloader/     Single-URL download logic
internal/worker/         Concurrent worker pool
configs/                 Sample / your own URL lists
downloads/               Where files are saved
```

## Requirements

- [Go](https://go.dev/dl/) 1.21+ installed
- Network access to the URLs you want to download

> **Windows note:** If you see *“Application Control policy has blocked this file”*, Smart App Control is blocking newly built unsigned programs. Turn it **Off** under  
> **Settings → Privacy & security → Windows Security → App & browser control → Smart App Control**, then try again.

## How to use

You do **not** have to create a separate `.exe` first. `go run` compiles and runs in one step.

### 1. Download from a URL list file (recommended)

`configs/urls.example.txt` is only a sample. You can edit it, or make your own file (any name) with **one URL per line**. Lines starting with `#` are ignored.

Example file:

```text
# my downloads
https://example.com/photo.png
https://example.com/report.pdf
```

Run it:

```powershell
go run ./cmd/downloader -file configs/urls.example.txt
```

Or with your own file:

```powershell
go run ./cmd/downloader -file configs/my-urls.txt
```

### 2. Pass URLs on the command line

```powershell
go run ./cmd/downloader https://example.com/a.png https://example.com/b.pdf
```

### 3. Pass a comma-separated list

```powershell
go run ./cmd/downloader -urls "https://example.com/a.png,https://example.com/b.pdf"
```

## Useful flags

| Flag | Default | Meaning |
|------|---------|---------|
| `-file` | _(empty)_ | Path to a text file with one URL per line |
| `-urls` | _(empty)_ | Comma-separated URLs |
| `-out` | `downloads` | Folder to save files into |
| `-workers` | `4` | Max downloads running at the same time |
| `-timeout` | `60s` | Per-request HTTP timeout |
| `-dedupe` | `true` | Skip duplicate URLs and identical file hashes |
| `-progress` | `true` | Show live download progress on the terminal |

### Duplicate detection

With `-dedupe` (on by default):

1. **Same URL** in the input list → ignored before download starts  
2. **Same URL** already downloaded earlier → `SKIP duplicate URL` (uses `downloads/.fetchpool-index.json`)  
3. **Same file content** (SHA-256) as an existing download → `SKIP duplicate hash` and deletes the extra copy  

```powershell
# Turn dedupe off
go run ./cmd/downloader -dedupe=false -file configs/my-urls.txt
```

Examples:

```powershell
# 8 parallel downloads into a custom folder
go run ./cmd/downloader -workers 8 -out my-files -file configs/my-urls.txt

# Longer timeout for slow links
go run ./cmd/downloader -timeout 2m -file configs/my-urls.txt
```

## Build a reusable binary (optional)

If you want an `.exe` you can run without `go run`:

```powershell
go build -o bin/downloader.exe ./cmd/downloader
.\bin\downloader.exe -file configs/urls.example.txt
```

## Example output

```text
Downloading 3 file(s) with 3 worker(s) → downloads
OK    https://example.com/a.png
      → downloads\a.png (13.2 KB in 195ms)
OK    https://example.com/b.bin
      → downloads\b.bin (1.0 KB in 212ms)

Done in 213ms — 2 ok, 0 failed, 14.2 KB total
```

Files appear in the `downloads/` folder (or the path you set with `-out`).
