package auto

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/kirikiri2"
	"github.com/DarlingGoose/vntext/pkg/engine/rpgmaker"
	"github.com/DarlingGoose/vntext/pkg/runner/installer"
	"github.com/DarlingGoose/vntext/pkg/util"
)

func SelectEngine(dir string) (engine.Engine, error) {
	engineList := []engine.Engine{
		rpgmaker.New(),
		kirikiri2.New(),
	}
	for _, e := range engineList {
		if e.IsEngine(dir) {
			return e, nil
		}
	}
	if util.IsDir(dir) {
		ed, err := DetectEngineFromPath(context.Background(), dir)
		if err == nil && ed.Engine != "unknown" {
			d, _ := json.MarshalIndent(ed, "", "  ")
			println(string(d))
			return nil, engine.ErrNoEngineFound
		}

	}

	return nil, engine.ErrNoEngineFound
}

func SelectOrInstallEngine(ctx context.Context, path string) (engine.Engine, error) {
	e, err := SelectEngine(path)
	if err == nil {
		return e, nil
	}

	if !util.IsExeFile(path) {
		return nil, err
	}

	install, detectErr := DetectInstallerExe(ctx, path)
	if detectErr != nil {
		return nil, err
	}

	if !install.IsInstaller {
		return nil, err
	}

	result, installErr := installer.InstallWindowsExe(ctx, installer.InstallOptions{
		InstallerPath: path,
		PrefixPath:    filepath.Join(util.ConfigBaseDir(), "prefixes", install.Name),
		Backend:       installer.BackendWine,
	})
	if installErr != nil {
		return nil, installErr
	}

	for _, candidate := range result.Candidates {
		slog.Info("checking Candidates", "dir", candidate)
		e, err := SelectEngine(candidate)
		if err == nil {
			return e, nil
		}
	}

	return nil, fmt.Errorf(
		"%w: installer completed, but no engine found in prefix %s",
		engine.ErrNoEngineFound,
		result.PrefixPath,
	)
}
