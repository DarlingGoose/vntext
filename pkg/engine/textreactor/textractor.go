package textreactor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/gr/autorunner"
	"github.com/DarlingGoose/gr/installer"
	"github.com/DarlingGoose/tr/pkg/textractor"
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/enginerun"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

var _ engine.EngineV2 = &Client{}

type Client struct {
	ProgramName string

	textractorMu    sync.RWMutex
	textractorGames map[string]*textractor.Client
	managedGames    map[string]*game.Game
}

func (c *Client) GetTextractor(game *game.Game) *textractor.Client {
	if game == nil {
		return nil
	}
	c.textractorMu.RLock()
	defer c.textractorMu.RUnlock()
	if g, ok := c.textractorGames[game.Name]; ok {
		return g
	}
	return nil
}

func (c *Client) ManagedGames() []*game.Game {
	c.textractorMu.RLock()
	defer c.textractorMu.RUnlock()

	games := make([]*game.Game, 0, len(c.managedGames))
	for _, g := range c.managedGames {
		if g == nil {
			continue
		}
		copy := *g
		games = append(games, &copy)
	}
	return games
}

func (c *Client) Shutdown() error {
	c.textractorMu.Lock()
	defer c.textractorMu.Unlock()

	for gameName, tr := range c.textractorGames {
		slog.Info("shutting down game", "game", gameName)
		_ = tr.Close()
	}
	c.textractorGames = map[string]*textractor.Client{}
	c.managedGames = map[string]*game.Game{}
	return nil
}

func (c *Client) InstallHook(ctx context.Context, game *game.Game) error {
	return nil // not needed, is installed at runtime
}

func (c *Client) IsEngine(dir string) bool {
	//todo idk how to do this for textractor
	_, err := util.FindExe(dir)
	if err != nil {
		return false
	}
	return true
}

func (c *Client) AddGame(ctx context.Context, fp string) (*game.Game, error) {
	exe, err := util.FindExe(fp)
	if err != nil {
		return nil, err
	}
	if ok, _ := installer.IsInstaller(exe); ok {
		return nil, fmt.Errorf("is installer, please install game first")
	}

	resolvedPath, err := filepath.Abs(strings.TrimSpace(fp))
	if err != nil {
		return nil, fmt.Errorf("resolve input path: %w", err)
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("stat input path: %w", err)
	}

	sourceRoot := resolvedPath
	if !info.IsDir() {
		sourceRoot = filepath.Dir(resolvedPath)
	}

	name := util.DeriveGameName(resolvedPath, exe, true)
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(fp)
	}
	prefix, err := enginerun.PrefixPath(c.ProgramName, "", name)
	if err != nil {
		return nil, err
	}

	arch, _ := autorunner.DetectFileArch(exe)
	g := &game.Game{
		Name:          name,
		GamePath:      sourceRoot,
		Executable:    exe,
		Architecture:  game.WineArchToArchitecture(arch),
		WorkingDir:    sourceRoot,
		Runner:        game.RunnerWine,
		PrefixPath:    prefix,
		RequiresSteam: false,
		CreatedAt:     time.Now().UTC(),
		Locale:        "",
		StageToPrefix: true,
		EngineName:    c.Name(),
	}

	if err := enginerun.ConfigureRunner(g); err != nil {
		return nil, err
	}
	options := gr.ApplyOptions(g.RunnerConfig.Options()...)

	g.EnvVars = game.EnvStringToEnv(options.Envs())
	i := textractor.Installer{}
	if _, err := i.Install(ctx, textractor.InstallOptions{
		WinePrefix: g.PrefixPath,
		Force:      true,
	}); err != nil {
		return nil, err
	}
	slog.Info("installed textreactor installer")
	return g, nil
}

func (c *Client) RunGame(ctx context.Context, g *game.Game) (*gr.Process, error) {

	return enginerun.RunGame(ctx, g)
}

func (c *Client) StopGame(ctx context.Context, proc *gr.Process) (*gr.Process, error) {
	return enginerun.StopGame(ctx, proc)
}

func (c *Client) GetFile(g *game.Game, file string) (*engine.EngineFileInfo, error) {
	return enginerun.UnsupportedFile(g, file)
}

func (c *Client) FollowGameText(ctx context.Context, g *game.Game, opts ...engine.FollowGameOptions) (chan engine.Line, error) {
	if g == nil {
		return nil, fmt.Errorf("game is nil")
	}
	client := c.GetTextractor(g)
	if client == nil {
		var err error
		client, err = c.attachTextractor(ctx, g)
		if err != nil {
			return nil, err
		}
		slog.Info("attached a new text reactor client")
	}

	cfg := mergeFollowOptions(opts)
	lines := c.textractorLines(ctx, client, g.TextHookFilter, cfg.History)
	return emitTextractorLines(ctx, lines, cfg.Filters), nil
}

func (c *Client) attachTextractor(ctx context.Context, g *game.Game) (*textractor.Client, error) {
	lister := textractor.ProcessLister{
		WinePrefix: g.PrefixPath,
	}

	var procs []textractor.WineProcess
	var err error
	maxTries := 10
	for {
		slog.Info("attempting to find process")
		procs, err = lister.FindByName(ctx, filepath.Base(g.Executable))
		if err == nil {
			break
		}
		if maxTries <= 0 {
			return nil, err
		}
		maxTries--
		time.Sleep(5 * time.Second)

	}

	if len(procs) == 0 {
		return nil, fmt.Errorf("game process not found for %s", filepath.Base(g.Executable))
	}
	arch, err := textractor.DetectArchFromFileCommand(ctx, g.Executable)
	if err != nil {
		arch = textractor.ArchX86
	}
	client, err := textractor.NewClient(textractor.ClientOptions{
		WinePrefix: g.PrefixPath,
		Arch:       arch,
	})
	if err != nil {
		return nil, err
	}
	pid := procs[0].PID
	if err := client.Attach(ctx, pid); err != nil {
		return nil, err
	}

	c.textractorMu.Lock()
	defer c.textractorMu.Unlock()
	if c.textractorGames == nil {
		c.textractorGames = map[string]*textractor.Client{}
	}
	if c.managedGames == nil {
		c.managedGames = map[string]*game.Game{}
	}
	gameCopy := *g
	c.textractorGames[g.Name] = client
	c.managedGames[g.Name] = &gameCopy
	return client, nil
}

func (c *Client) textractorLines(ctx context.Context, client *textractor.Client, filters []string, history bool) <-chan *textractor.Line {
	filters = normalizeHookGroups(filters)
	if len(filters) == 1 {
		return client.HookFeed(ctx, filters[0], history)
	}
	if len(filters) > 1 {
		return textractor.FilterLines(client.Lines(), textractor.NewHookFilter(filters...))
	}
	if history {
		return client.HookFeed(ctx, "", true)
	}
	return client.Lines()
}

func emitTextractorLines(ctx context.Context, lines <-chan *textractor.Line, filters []func(*engine.Line) *engine.Line) chan engine.Line {
	output := make(chan engine.Line, 100)
	go func() {
		defer close(output)
		for l := range lines {
			if l == nil {
				continue
			}
			line := &engine.Line{
				Raw:     l.Raw,
				Hook:    l.Hook,
				Text:    l.Text,
				Speaker: l.Speaker,
			}
			for _, filter := range filters {
				if filter != nil {
					line = filter(line)
				}
				if line == nil {
					break
				}
			}
			if line == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case output <- *line:
			}
		}
	}()
	return output
}

func mergeFollowOptions(opts []engine.FollowGameOptions) engine.FollowGameOptions {
	var cfg engine.FollowGameOptions
	for _, opt := range opts {
		if opt.MaxLines > 0 {
			cfg.MaxLines = opt.MaxLines
		}
		if opt.History {
			cfg.History = true
		}
		cfg.Filters = append(cfg.Filters, opt.Filters...)
	}
	return cfg
}

func normalizeHookGroups(groups []string) []string {
	out := make([]string, 0, len(groups))
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, part := range strings.Split(group, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			part = textractor.HookGroup(part)
			if _, ok := seen[part]; ok {
				continue
			}
			out = append(out, part)
			seen[part] = struct{}{}
		}
	}
	return out
}

func (c *Client) Name() string {
	return "textreactor"
}

func textReactorRunGame(g *game.Game) *game.Game {
	if g == nil {
		return nil
	}

	copy := *g
	if copy.Runner == "" || textReactorShouldDefaultToWine(&copy) {
		copy.Runner = game.RunnerWine
		copy.RunnerPath = textReactorWineBin(&copy)
	}
	return &copy
}

func textReactorShouldDefaultToWine(g *game.Game) bool {
	if g == nil || g.Runner != game.RunnerGamescope {
		return false
	}
	if strings.TrimSpace(g.RunnerPath) != "" {
		return false
	}
	return g.GamescopeConfig == nil
}

func textReactorWineBin(g *game.Game) string {
	if g == nil || g.WineConfig == nil {
		return ""
	}
	return strings.TrimSpace(g.WineConfig.WineBin)
}
