# vntext

`vntext` is a Go CLI for installing, launching, and extracting text from visual novels on Linux. It is centered on `EngineV2`: commands load an installed game config, select one engine instance, then call engine methods for setup, hook install, launch, stop, and text following.

The default program/config name is `vntext`. Code should use `pkg/app.Name()` instead of hardcoding that name.

## Features

- Detect supported engines and write installed-game configs.
- Launch games through [`gr`](https://github.com/DarlingGoose/gr) Wine/gamescope runners.
- Save per-game runner options, Wine options, gamescope options, and text hook filters.
- Install engine-specific text hooks.
- Follow extracted game text through `EngineV2.FollowGameText`.
- Support Textractor/TextReactor for games without native file hooks.
- Preserve Japanese file encodings and newlines when patching scripts.

## Build

```sh
go build -o vntext .
```

Run tests:

```sh
go test ./...
```

## Commands

### Install A Game

```sh
vntext install-game /path/to/game
```

Useful flags:

```sh
vntext install-game /path/to/game --print
vntext install-game /path/to/game --text-hook=false
vntext install-game /path/to/game --output ./game.json
```

Command flow:

1. `cmd/installgame.go` calls `gameConfig.InstallGame`.
2. `gameConfig.InstallGame` calls `auto.SelectEngineV2`.
3. The selected engine receives `AddGame(ctx, path)`.
4. If enabled, the selected engine receives `InstallHook(ctx, game)`.
5. The resulting `game.Game` is written as JSON.

### Install Or Refresh A Hook

```sh
vntext install-hook "Game Name"
```

Useful flags:

```sh
vntext install-hook "Game Name" --engine kirikiri2
vntext install-hook "Game Name" --hook-filter "@13F548:KSH_dl.exe"
vntext install-hook "Game Name" --clear-hook-filter
```

Command flow:

1. Load installed game configs.
2. Select an engine through `auto.DefaultEngineSelectorV2()`.
3. Call `EngineV2.InstallHook(ctx, game)`.
4. Save hook metadata and default text hook filters back to the game config.

`--hook-filter` is mainly for Textractor/TextReactor. It stores `game.TextHookFilter`, which `FollowGameText` uses by default.

### Run A Game

```sh
vntext run-game "Game Name"
```

With text output:

```sh
vntext run-game "Game Name" --sync --follow
```

Useful flags:

```sh
vntext run-game --list
vntext run-game "Game Name" --virtual-desktop 1280x720
vntext run-game "Game Name" --virtual-desktop off
vntext run-game "Game Name" --sync --follow --log-file /path/to/vntext.log
```

Command flow:

1. Load installed game configs.
2. Select an engine through the cached v2 selector.
3. Call `EngineV2.RunGame(ctx, game)`.
4. On `--sync`, wait for the returned `gr.Process`.
5. On `--follow`, call `EngineV2.FollowGameText(ctx, game)`.
6. On shutdown, call `EngineV2.StopGame(ctx, proc)`.

For Wine games, `gr.Process.WinePID` is used when available so `--sync` follows the actual Wine game process instead of only the host Wine launcher.

### Manage Runner Config

```sh
vntext runner-config "Game Name"
```

Set Wine options:

```sh
vntext runner-config "Game Name" \
  --runner wine \
  --wine-bin /usr/bin/wine \
  --wine-prefix ~/.config/vntext/prefixes/example \
  --arch win64 \
  --env LANG=ja_JP.UTF-8
```

Set gamescope options:

```sh
vntext runner-config "Game Name" \
  --runner gamescope \
  --gamescope-bin /usr/bin/gamescope \
  --resolution 1280x720 \
  --output-resolution 1280x720 \
  --fullscreen
```

Manage Textractor default filters:

```sh
vntext runner-config "Game Name" --text-hook-filter "@dialogue.dll:1234"
vntext runner-config "Game Name" --clear-text-hook-filter
```

Import/export combined vntext runner profiles:

```sh
vntext runner-config "Game Name" --export ./runner-profile.json
vntext runner-config "Game Name" --import ./runner-profile.json
```

Import/export native `gr` runner configs:

```sh
vntext runner-config "Game Name" --export-wine-config ./wine.json
vntext runner-config "Game Name" --import-wine-config ./wine.json
vntext runner-config "Game Name" --export-gamescope-config ./gamescope.json
vntext runner-config "Game Name" --import-gamescope-config ./gamescope.json
```

## Game Configs

Default location:

```text
~/.config/<program-name>/games/*.json
```

With the default program name:

```text
~/.config/vntext/games/*.json
```

Important fields in `game.Game`:

| JSON field | Purpose |
|---|---|
| `name` | Display name. |
| `game_path` | Game root. |
| `executable` | Executable to launch. |
| `working_dir` | Launch working directory. |
| `runner` | `wine` or `gamescope`. |
| `runner_path` | Primary runner binary. For Wine this is Wine; for gamescope this is gamescope. |
| `runner_config` | Common `gr.Config` launch options. |
| `wine_config` | Native `gr/wine.Options`. |
| `gamescope_config` | Native `gr/gamescope.Options`. |
| `prefix_path` | Wine prefix path. |
| `virtual_desktop` | Wine desktop size. `off` disables it. |
| `locale` | Wine locale override. Empty means detect with `autorunner.DetectWineLang(executable)`. |
| `text_hook_log_file` | Log file used by file-hook engines. |
| `text_hook_filter` | Default Textractor hook groups to follow. |
| `engine_name` | Engine used for this config. |

## Supported Engines

| Engine | Package | AddGame | InstallHook | RunGame | FollowGameText |
|---|---|---:|---:|---:|---:|
| Kirikiri2 | `pkg/engine/kirikiri2` | Yes | TJS/log.exe hook | Wine/gamescope through `enginerun` | Follows `vntext.log` |
| RPG Maker MV/MZ | `pkg/engine/rpgmaker` | Yes | JavaScript plugin hook | Wine/gamescope through `enginerun` | Follows plugin log |
| TextReactor/Textractor | `pkg/engine/textreactor` | Yes | Runtime Textractor install | Defaults to Wine | Follows Textractor client |

## EngineV2

Commands should call `EngineV2`, not legacy engine/runner APIs.

Current interface:

```go
type EngineV2 interface {
	Name() string
	IsEngine(dir string) bool

	AddGame(ctx context.Context, filepath string) (*game.Game, error)
	InstallHook(ctx context.Context, game *game.Game) error
	RunGame(ctx context.Context, game *game.Game) (*gr.Process, error)
	StopGame(ctx context.Context, proc *gr.Process) (*gr.Process, error)

	GetFile(g *game.Game, file string) (*engine.EngineFileInfo, error)
	FollowGameText(ctx context.Context, game *game.Game, opts ...engine.FollowGameOptions) (chan engine.Line, error)

	Shutdown() error
	ManagedGames() []*game.Game
	GetTextractor(game *game.Game) *textractor.Client
}
```

### Command-To-Method Mapping

| Command | EngineV2 methods called |
|---|---|
| `install-game` | `IsEngine`, `AddGame`, optionally `InstallHook` |
| `install-hook` | `InstallHook` |
| `run-game` | `RunGame`, optionally `FollowGameText`, `StopGame` |
| `runner-config` | Does not call engine methods; edits saved `game.Game` runner/text fields |

## Implementing An EngineV2

Use the existing engines as templates:

- `pkg/engine/kirikiri2/v2.go`
- `pkg/engine/rpgmaker/v2.go`
- `pkg/engine/textreactor/textractor.go`

Minimum implementation shape:

```go
package myengine

import (
	"context"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/tr/pkg/textractor"
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/enginerun"
	"github.com/DarlingGoose/vntext/pkg/game"
)

type Engine struct{}

var _ engine.EngineV2 = (*Engine)(nil)

func (e *Engine) Name() string { return "myengine" }

func (e *Engine) IsEngine(path string) bool {
	// Detect by files, directories, executable metadata, etc.
	return false
}

func (e *Engine) AddGame(ctx context.Context, path string) (*game.Game, error) {
	g := &game.Game{
		Name:        "Example",
		GamePath:    path,
		Executable:  "/path/to/Game.exe",
		WorkingDir:  "/path/to",
		Runner:      game.RunnerWine,
		PrefixPath:  "/path/to/prefix",
		EngineName:  e.Name(),
		Locale:      "",
	}

	if err := enginerun.ConfigureRunner(g); err != nil {
		return nil, err
	}

	return g, nil
}

func (e *Engine) InstallHook(ctx context.Context, g *game.Game) error {
	// Patch files, install plugins, or no-op for runtime hook engines.
	return nil
}

func (e *Engine) RunGame(ctx context.Context, g *game.Game) (*gr.Process, error) {
	return enginerun.RunGame(ctx, g)
}

func (e *Engine) StopGame(ctx context.Context, proc *gr.Process) (*gr.Process, error) {
	return enginerun.StopGame(ctx, proc)
}

func (e *Engine) FollowGameText(ctx context.Context, g *game.Game, opts ...engine.FollowGameOptions) (chan engine.Line, error) {
	return enginerun.FollowGameText(ctx, g, opts...)
}

func (e *Engine) GetFile(g *game.Game, file string) (*engine.EngineFileInfo, error) {
	return enginerun.UnsupportedFile(g, file)
}

func (e *Engine) Shutdown() error { return nil }
func (e *Engine) ManagedGames() []*game.Game { return nil }
func (e *Engine) GetTextractor(g *game.Game) *textractor.Client { return nil }
```

### AddGame Responsibilities

`AddGame` should return a complete runnable `game.Game`.

Set at least:

- `Name`
- `GamePath`
- `Executable`
- `WorkingDir`
- `Runner`
- `PrefixPath`
- `EngineName`
- `CreatedAt`
- `TextHookLogFile` if the engine follows a file

Then call:

```go
err := enginerun.ConfigureRunner(g)
```

`ConfigureRunner` uses `gr/autorunner.AutoOptionsForExe` and `autorunner.DetectWineLang`. If `game.Locale` is empty, the executable is scanned for Wine locale markers/resources. If `game.Locale` is set, it wins over stored runner envs.

### InstallHook Responsibilities

`InstallHook` should be idempotent. Running it repeatedly should not duplicate script patches or plugin entries.

For file-patching engines:

- Detect the target script/plugin file.
- Preserve encoding and line endings using `pkg/textfile`.
- Update `g.TextHookLogFile`.

For runtime hook engines such as TextReactor:

- Install required runtime files if needed.
- It can be a no-op if attachment happens in `FollowGameText`.

### RunGame And StopGame

Most Wine-backed engines should delegate:

```go
return enginerun.RunGame(ctx, g)
```

and:

```go
return enginerun.StopGame(ctx, proc)
```

`enginerun.RunGame` handles:

- Wine target/args.
- Wine virtual desktop.
- Wine/gamescope runner selection.
- Saved `runner_config`, `wine_config`, and `gamescope_config`.
- Locale env merging.

`enginerun.StopGame` prefers `gr.Process.WinePID` when available, so it stops the Wine process instead of accidentally killing an unrelated Linux PID.

### FollowGameText Responsibilities

`FollowGameText` should return normalized `engine.Line` values.

For file-backed hooks:

```go
return enginerun.FollowGameText(ctx, g, opts...)
```

For Textractor/TextReactor:

- Locate the running Wine game process.
- Attach a `textractor.Client`.
- Use `game.TextHookFilter` as the default hook group filter.
- Convert `textractor.Line` to `engine.Line`.

`run-game --sync --follow` calls this method. Do not make the command layer tail files directly.

### SelectorV2

Engine instances are created once by the cached selector:

```go
selector := auto.DefaultEngineSelectorV2()
eng, err := selector.Select(path)
```

By name:

```go
eng := selector.ByName("kirikiri2")
```

To register a new engine, add it in `NewEngineSelectorV2` in `pkg/engine/auto/auto.go`:

```go
engines: []engine.EngineV2{
	rpgmaker.New(),
	kirikiri2.New(),
	&textreactor.Client{ProgramName: programName},
	myengine.New(),
},
```

The selector should own one instance of each engine. This matters for engines such as TextReactor that keep attached clients in memory.

## Text And Encoding

Japanese game scripts often use Shift-JIS/CP932 and CRLF. Use `pkg/textfile` for script edits:

```go
err := textfile.Update("data/startup.tjs", func(s string, style textfile.FileStyle) (string, error) {
	line := `Scripts.execStorage("text_logger.tjs");`
	if strings.Contains(s, line) {
		return s, nil
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s + line + "\n", nil
})
```

Avoid `os.WriteFile` for script patching unless the file is known to be UTF-8 and newline-insensitive.

## Debugging

No text from `--sync --follow`:

1. Confirm `run-game` selected the expected engine.
2. Confirm `TextHookLogFile` exists for file-backed engines.
3. Confirm `EngineV2.FollowGameText` is being used.
4. For Wine games, confirm `gr.Process.WinePID` is populated.
5. For Textractor, confirm `TextHookFilter` matches the hook group.

Wrong locale:

1. Check `game.Locale`.
2. If empty, `autorunner.DetectWineLang(game.Executable)` should detect the locale.
3. Check saved `runner_config.envs`; `game.Locale` or detected locale should override stale `C.UTF-8`.

Useful commands:

```sh
vntext runner-config "Game Name"
file /path/to/startup.tjs
find . -iname '*.tjs' -o -iname '*.ks'
find ~/.config/vntext/games -name '*.json'
```

## Project Layout

```text
cmd/                         CLI commands
pkg/app                      Program name defaults
pkg/game                     Installed game config structs
pkg/gameConfig               Load/save installed game configs
pkg/engine                   Engine interfaces and shared types
pkg/engine/auto              EngineV2 selector
pkg/engine/enginerun         Shared Wine/gamescope/follow helpers
pkg/engine/kirikiri2         Kirikiri2 detection/hook support
pkg/engine/rpgmaker          RPG Maker detection/hook support
pkg/engine/textreactor       Textractor/TextReactor support
pkg/textfile                 Encoding/newline preserving file edits
pkg/util                     Shared filesystem/path helpers
```

## Current Status

| Area | Status |
|---|---|
| EngineV2 command flow | Active path |
| Kirikiri2 | Supported |
| RPG Maker MV/MZ | Supported |
| TextReactor/Textractor | Supported |
| Wine runner config | Supported |
| Gamescope runner config | Supported |
| Encoding-preserving patching | Supported |
