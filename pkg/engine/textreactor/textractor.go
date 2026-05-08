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
}

func (c *Client) GetTextractor(game *game.Game) *textractor.Client {
	c.textractorMu.RLock()
	defer c.textractorMu.RUnlock()
	if g, ok := c.textractorGames[game.Name]; ok {
		return g
	}
	return nil
}

func (c *Client) ManagedGames() []*game.Game {
	return nil
}

func (c *Client) Shutdown() error {
	for gameName, tr := range c.textractorGames {
		slog.Info("shutting down game", "game", gameName)
		_ = tr.Close()
	}
	c.textractorGames = map[string]*textractor.Client{}
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
		Force:      false,
	}); err != nil {
		return nil, err
	}

	return g, nil
}

func (c *Client) RunGame(ctx context.Context, game *game.Game) (*gr.Process, error) {
	return enginerun.RunGame(ctx, game)
}

func (c *Client) StopGame(ctx context.Context, proc *gr.Process) (*gr.Process, error) {
	return enginerun.StopGame(ctx, proc)
}

func (c *Client) GetFile(g *game.Game, file string) (*engine.EngineFileInfo, error) {
	return enginerun.UnsupportedFile(g, file)
}

func (c *Client) FollowGameText(ctx context.Context, game *game.Game, opts ...engine.FollowGameOptions) (chan engine.Line, error) {
	lister := textractor.ProcessLister{
		WinePrefix: game.PrefixPath,
	}

	procs, err := lister.FindByName(ctx, filepath.Base(game.Executable))
	if err != nil {
		return nil, err
	}
	arch, err := textractor.DetectArchFromFileCommand(ctx, game.Executable)
	if err != nil {
		arch = textractor.ArchX86
	}
	client, err := textractor.NewClient(textractor.ClientOptions{
		WinePrefix: game.PrefixPath,
		Arch:       arch,
	}) //todo need to close client?
	if err != nil {
		return nil, err
	}
	pid := procs[0].PID
	if err := client.Attach(ctx, pid); err != nil {
		return nil, err
	}
	//todo need a way to laod history
	//todo need a way to save history to file
	//todo need a way to filter by hook id
	c.textractorMu.Lock()
	defer c.textractorMu.Unlock()
	if c.textractorGames == nil {
		c.textractorGames = map[string]*textractor.Client{}

	}
	c.textractorGames[game.Name] = client
	client.Lines()
	return nil, nil
}

func (c *Client) Name() string {
	return "textreactor"
}
