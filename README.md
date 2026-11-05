# Concurrent Downloader (with a simple LLM agent)

A small Go CLI that downloads multiple files from the internet **at the same time**, with an optional **simple LLM agent**.

You can pass URLs yourself as usual, or type a plain-English request like *“download 3 sample images”*. A large language model (LLM) proposes the URLs, then this app downloads them with the same worker pool, progress, and duplicate checks.

Instead of fetching links one after another, it uses a worker pool so several downloads can run in parallel. You control how many run at once with the `-workers` flag. Finished files are saved to a local folder (by default `downloads/`).

## What this project does

- Accepts a list of HTTP/HTTPS URLs
- Downloads them concurrently (configurable worker count)
- Saves each file under an output directory
- Avoids overwriting existing files by appending `-1`, `-2`, etc.
- **Duplicate detection** — skips the same URL again, and skips saving when content SHA-256 already exists
- **Live progress** — shows active downloads with % (when known), bytes, and speed
- **Simple LLM agent** — natural language → proposed URLs → download
- **Max file size** — skip downloads larger than a limit (e.g. `-max-size 5MB`)
- Prints success / skip / failure for each URL, plus totals at the end

### Project layout

```
cmd/downloader/          CLI entrypoint
internal/agent/          Simple LLM download planner (-agent)
internal/downloader/     Single-URL download logic
internal/worker/         Concurrent worker pool
configs/                 Sample / your own URL lists
downloads/               Where files are saved
```

## Requirements

- [Go](https://go.dev/dl/) 1.21+ installed
- Network access to the URLs you want to download
- For `-agent` only: an OpenAI API key (details below)

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

### 4. Simple LLM agent (natural language → URLs → download)

This mode uses a simple LLM call (OpenAI-compatible chat API). The model only **plans URLs**; your Go downloader still does the real work (concurrency, progress, dedupe).

#### How to get an OpenAI API key

1. Go to [https://platform.openai.com](https://platform.openai.com) and sign in (or create an account).
2. Open **API keys**: [https://platform.openai.com/api-keys](https://platform.openai.com/api-keys)
3. Click **Create new secret key** and copy it (it starts with `sk-...`).
4. Add a little **billing credit** if the account asks for it — API calls usually need that.
5. Keep the key private. Do **not** commit it to GitHub or paste it into public chats.

#### Set the key (PowerShell)

```powershell
$env:OPENAI_API_KEY = "sk-..."

# optional:
# $env:OPENAI_MODEL = "gpt-4o-mini"
# $env:OPENAI_BASE_URL = "https://api.openai.com/v1"
```

`OPENAI_BASE_URL` lets you use other OpenAI-compatible providers if you want.

#### Run the agent

Dry-run (plan only — shows URLs, does not download):

```powershell
go run ./cmd/downloader -agent "download 3 small sample images" -agent-dry-run
```

Plan + download:

```powershell
go run ./cmd/downloader -agent "download 2 sample PDFs" -agent-limit 2
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
| `-agent` | _(empty)_ | Natural-language prompt for the simple LLM planner |
| `-agent-dry-run` | `false` | Show proposed URLs only; do not download |
| `-agent-limit` | `5` | Max URLs the agent may propose |
| `-max-size` | `0` | Skip files larger than this (`5MB`, `500KB`, bytes); `0` = unlimited |

```powershell
# Skip anything larger than 1 MB
go run ./cmd/downloader -max-size 1MB -file configs/urls.example.txt
```

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
