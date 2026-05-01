# vntext

`vntext` is a CLI tool for installing, launching, and extracting text from visual novels and Japanese games.

The main goal is to make it easier to run games through Wine or Proton on Linux while also supporting engine-specific text logging hooks. It is designed around small reusable Go packages so the CLI can install games, detect engines, patch scripts, preserve original file encodings, launch games, and collect extracted text logs.

## Overview

`vntext` can:

- Detect supported game engines
- Install and stage games into Wine or Proton prefixes
- Preserve original Japanese script encodings when patching files
- Install text logging hooks where supported
- Build and stage a tiny `log.exe` helper
- Launch games with a configured runner
- Check whether a configured game is already running
- Append extracted text to log files

## Supported Engines

| Engine | Detection | Text Hook Support | Notes |
|---|---:|---:|---|
| Kirikiri 2 | Supported | Supported | Uses TJS/KAG script hooks |
| RPG Maker | Supported | Partial / Planned | MV/MZ-style detection |
| Unknown | Supported fallback | Not supported | Can still be installed/run |

Kirikiri 2 is currently the best-supported engine. The text hook works by patching or loading TJS script files and writing captured text to a log file.

RPG Maker detection exists, but text extraction support depends on the specific game/runtime and may require additional JavaScript injection or engine-specific hooks.

## Installation

Build the CLI:

```bash
go build -o vntext .
````

Or install it locally:

```bash
go install .
```

Then verify:

```bash
vntext --help
```

## Basic Usage

Install a game from a directory:

```bash
vntext install-game /path/to/game
```

Print the detected install plan without writing config:

```bash
vntext install-game --print /path/to/game
```

Run an installed game:

```bash
vntext run-game "Game Name"
```

If no game name or flags are provided, `run-game` can show an interactive TUI selector using the default config location:

```bash
vntext run-game
```

The selector lists previously installed games and allows one to be launched.

## Game Configs

Installed games are saved as JSON config files in the default config/cache location used by `vntext`.

A game config includes information such as:

| Field           | Description                             |
| --------------- | --------------------------------------- |
| `Name`          | Display name for the game               |
| `Runner`        | Runner type, such as `wine` or `proton` |
| `Executable`    | Path to the game executable             |
| `Prefix`        | Wine/Proton prefix path                 |
| `Engine`        | Detected engine type                    |
| `RequiresSteam` | Whether Steam is required               |
| `SteamAppID`    | Steam app id, if applicable             |
| `IconPath`      | Optional icon path                      |
| `ImagePath`     | Optional preview image path             |

## Text Logging

For supported engines, `vntext` can install a text logger hook.

The general flow is:

1. Detect the game engine.
2. Find the relevant startup or system script.
3. Patch the script while preserving its original encoding and newline style.
4. Build or stage a small `log.exe` helper.
5. Launch the game.
6. Game text is appended to a log file.

For Kirikiri 2 games, many script files are encoded as Shift-JIS or CP932 with CRLF line endings. `vntext` preserves these formats when patching files.

## Encoding Preservation

Japanese game script files often look like this when inspected with `file`:

```text
data/startup.tjs: ASCII text, with CRLF line terminators
data/system/YesNoDialog.tjs: JavaScript source, Non-ISO extended-ASCII text, with CRLF line terminators
```

The second file is often Shift-JIS or CP932.

`vntext` uses the `textfile` package to:

* Detect ASCII, UTF-8, UTF-8 BOM, UTF-16 BOM, Shift-JIS/CP932, and EUC-JP
* Detect LF, CRLF, or CR line endings
* Decode text to UTF-8 internally
* Write files back using their original encoding and newline style

This prevents editors or patching code from accidentally converting Japanese scripts into UTF-8 or changing CRLF to LF.

## Using the Internal Packages

The CLI is built from reusable packages. These packages can also be used directly by other tools inside the repo.

## `pkg/textfile`

Use this package when editing Japanese game files.

### Read a file, modify text, and preserve encoding/newlines

```go
tf, err := textfile.Read("data/startup.tjs")
if err != nil {
	return err
}

updated := tf.Text

hook := `Scripts.execStorage("text_logger.tjs");`
if !strings.Contains(updated, hook) {
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += hook + "\n"
}

return textfile.Write("data/startup.tjs", updated, tf.Style, 0644)
```

### Update in place

```go
err := textfile.Update("data/startup.tjs", func(s string, style textfile.FileStyle) (string, error) {
	line := `Scripts.execStorage("text_logger.tjs");`

	if strings.Contains(s, line) {
		return s, nil
	}

	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}

	s += line + "\n"
	return s, nil
})
```

### Replace text while preserving the original file style

```go
err := textfile.Replace(
	"data/system/YesNoDialog.tjs",
	"System.inform(\"old\");",
	"System.inform(\"new\");",
	1,
)
```

## `pkg/engine`

Use this package for engine detection.

Expected responsibilities:

* Identify the engine used by a game directory
* Return engine metadata
* Choose the correct installer or hook implementation

Example shape:

```go
detected, err := engine.Detect(gameDir)
if err != nil {
	return err
}

fmt.Println("engine:", detected.Name)
```

Engine detection should be used before installing hooks because each engine needs a different strategy.

## `pkg/engine/kirikiri2`

Use this package for Kirikiri-specific install and hook behavior.

Expected responsibilities:

* Detect Kirikiri 2 games
* Locate `startup.tjs` or other relevant scripts
* Stage extracted or patched files
* Install TJS/KAG text logger hooks
* Preserve Shift-JIS/CP932 and CRLF formatting

Example shape:

```go
installer := kirikiri2.NewInstaller()

err := installer.Install(ctx, gameConfig)
if err != nil {
	return err
}
```

For Kirikiri 2, script patching should go through `pkg/textfile` instead of `os.ReadFile` / `os.WriteFile` directly.

## `pkg/engine/kirikiri2/log`

Use this package for building or staging the log helper.

The log helper is a small executable that receives text from the game and appends it to a log file.

Example build command used internally:

```go
cmd := exec.CommandContext(
	ctx,
	"go",
	"build",
	"-trimpath",
	"-ldflags=-H=windowsgui -s -w",
	"-o",
	tmpOut,
	".",
)

cmd.Dir = logDir
cmd.Env = append(os.Environ(),
	"GOOS=windows",
	"GOARCH=amd64",
)
```

The important part is that the command runs from the temporary log helper source directory:

```go
cmd.Dir = logDir
```

and builds:

```text
.
```

instead of passing the temp directory as the package argument.

## `pkg/game`

Use this package for game configuration structs and config persistence.

Expected responsibilities:

* Represent installed games
* Store runner, executable, prefix, engine, icon, image, and Steam fields
* Load and save game configs
* Derive normalized game names

Example shape:

```go
cfg := &game.Game{
	Name:       "Example Game",
	Runner:     "proton",
	Executable: "/path/to/Game.exe",
	Prefix:     "/path/to/prefix",
	Engine:     "kirikiri2",
}

err := game.SaveConfig(cfg)
if err != nil {
	return err
}
```

## `pkg/runner`

Use this package to launch games and check process status.

Runner interface:

```go
type Runner interface {
	Run(game *game.Game) (*ProcessStatus, error)
	RunBackground(game *game.Game) (*ProcessStatus, error)
	IsRunning(game *game.Game) (*ProcessStatus, error)
	Stop(p *ProcessStatus)
}
```

Process status:

```go
type ProcessStatus struct {
	PID     int
	Message string
	Status  int
}
```

Example usage:

```go
r := runner.NewProtonRunner()

status, err := r.RunBackground(cfg)
if err != nil {
	return err
}

fmt.Println("started pid:", status.PID)
```

Checking whether a game is already running:

```go
status, err := r.IsRunning(cfg)
if err != nil {
	return err
}

if status.Status == 1 {
	fmt.Println("game is running:", status.PID)
}
```

## `pkg/util`

Use this package for shared helpers that do not belong to a specific engine, runner, or game config package.

Good candidates for `pkg/util`:

* Path normalization
* File existence checks
* Directory creation
* Safe copy helpers
* Slug generation
* Executable discovery

## Common Workflows

Install a Kirikiri 2 game:

```bash
vntext install-game /path/to/game
```

Print install details first:

```bash
vntext install-game --print /path/to/game
```

Run a game by name:

```bash
    vntext run-game "testname"
```


Run the interactive selector:

```bash
vntext run-game
```

Check generated logs:

```bash
find ~/.cache -iname 'vntext.log' -o -iname 'krkr.console.log'
```

## Debugging

If a hook is installed but no text appears, check:

1. The game engine was detected correctly.
2. The patched script was actually loaded by the game.
3. The patched file preserved its original encoding.
4. The game can execute `log.exe`.
5. The log file path is writable.
6. Wine or Proton did not block the helper executable.
7. The game text is not rendered only as images.

Useful commands:

```bash
file data/startup.tjs
```

```bash
find . -iname '*.tjs' -o -iname '*.ks'
find . -iname '*console*' -o -iname '*log*'
```

## Development

Run all tests:

```bash
go test ./...
```

Run only textfile tests:

```bash
go test ./pkg/textfile
```

Build the CLI:

```bash
go build -o vntext .
```


## Design Notes

The project should avoid directly editing Japanese script files with `os.ReadFile` and `os.WriteFile` unless the file is known to be UTF-8.

Use `pkg/textfile` for script patching because many older Japanese games use Shift-JIS/CP932 and CRLF line endings.

The CLI should keep engine-specific behavior inside engine packages. The command layer should mostly coordinate:

1. Load config.
2. Detect engine.
3. Install or patch through the engine package.
4. Save config.
5. Run through the runner package.

This keeps the command code small and makes it easier to add new engines later.

## Project Status

| Area        | Status                      |
| ----------- |-----------------------------|
| CLI install | Working / in progress       |
| CLI run     | Working / in progress       |
| TUI select  | Planned / in progress       |
| Kirikiri 2  | Supported                   |
| RPG Maker   | Supported |
| Textfile    | Encoding preservation       |
| Log helper  | Working                     |

