package gameConfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

func FindInstalledGame(games []*game.Game, query string) (*game.Game, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("game name is required")
	}

	wanted := util.SanitizeName(query)

	var matches []*game.Game

	for _, g := range games {
		if g == nil {
			continue
		}

		name := strings.TrimSpace(g.Name)
		cleanName := util.SanitizeName(name)

		switch {
		case strings.EqualFold(name, query):
			matches = append(matches, g)
		case strings.EqualFold(cleanName, wanted):
			matches = append(matches, g)
		case strings.Contains(strings.ToLower(name), strings.ToLower(query)):
			matches = append(matches, g)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("game %q not found", query)
	}

	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, match := range matches {
			names = append(names, match.Name)
		}
		sort.Strings(names)

		return nil, fmt.Errorf("game %q is ambiguous; matched: %s", query, strings.Join(names, ", "))
	}

	return matches[0], nil
}

func LoadInstalledGames(configDirs ...string) ([]*game.Game, error) {
	var games []*game.Game
	d := map[string]struct{}{}
	for _, dir := range configDirs {
		foundGames, _ := loadInstalledGames(dir)

		for _, f := range foundGames {
			if _, ok := d[f.GamePath]; ok {
				continue
			}
			games = append(games, f)
			d[f.GamePath] = struct{}{}
		}
	}

	return games, nil
}

func loadInstalledGames(configDir string) ([]*game.Game, error) {
	configDir = strings.TrimSpace(configDir)

	entries, err := os.ReadDir(configDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read game config dir: %w", err)
	}

	var games []*game.Game

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}

		path := filepath.Join(configDir, entry.Name())

		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read game config %s: %w", path, err)
		}

		var g game.Game
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("parse game config %s: %w", path, err)
		}

		if strings.TrimSpace(g.Name) == "" {
			g.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}

		games = append(games, &g)
	}

	sort.Slice(games, func(i, j int) bool {
		return strings.ToLower(games[i].Name) < strings.ToLower(games[j].Name)
	})

	return games, nil
}
