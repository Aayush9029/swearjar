<p align="center">
  <img src="assets/icon.png" width="128" alt="swearjar">
  <h1 align="center">swearjar</h1>
  <p align="center">A terminal swear jar for your coding agent transcripts</p>
</p>

<p align="center">
  <a href="https://github.com/Aayush9029/swearjar/releases/latest"><img src="https://img.shields.io/github/v/release/Aayush9029/swearjar" alt="Release"></a>
  <a href="https://github.com/Aayush9029/swearjar/blob/main/LICENSE"><img src="https://img.shields.io/github/license/Aayush9029/swearjar" alt="License"></a>
</p>

<p align="center">
  <img src="assets/demo.gif" alt="swearjar demo" width="800">
</p>

## Install

```bash
brew install aayush9029/tap/swearjar
```

Or tap first:

```bash
brew tap aayush9029/tap
brew install swearjar
```

## Usage

```bash
swearjar                         # open the interactive TUI
swearjar scan                    # print a compact report
swearjar scan --agent codex      # scan one agent
swearjar scan --week             # scan the last 7 days
swearjar scan ./chat.jsonl       # scan files or folders
```

Uses DuckDB for local aggregation and go-away for profanity detection.

## License

MIT
