package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/auto"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
	"github.com/spf13/cobra"
)

type InstallOptions struct {
	TextHook bool
	Output   string
	Print    bool
	NoSave   bool
}

func NewInstallCommand() *cobra.Command {
	var opts InstallOptions

	cmd := &cobra.Command{
		Use:   "install-game [path]",
		Short: "Auto-detect and install a visual novel game",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Output = strings.TrimSpace(opts.Output)

			g, eng, err := InstallGame(args[0], opts)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "installed game\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  engine:     %s\n", eng.Name())
			fmt.Fprintf(cmd.OutOrStdout(), "  name:       %s\n", g.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  executable: %s\n", g.Executable)
			fmt.Fprintf(cmd.OutOrStdout(), "  workingDir: %s\n", g.WorkingDir)
			if g.TextHookLogFile != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  text log:   %s\n", g.TextHookLogFile)
			}

			if opts.Output != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  config:     %s\n", opts.Output)
			}

			if opts.Print {
				raw, err := json.MarshalIndent(g, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(
		&opts.TextHook,
		"text-hook",
		true,
		"install text hook after game setup",
	)

	cmd.Flags().StringVarP(
		&opts.Output,
		"output",
		"o",
		"",
		"write game config JSON to this path",
	)

	cmd.Flags().BoolVar(
		&opts.Print,
		"print",
		false,
		"print installed game config JSON",
	)

	cmd.Flags().BoolVar(
		&opts.NoSave,
		"no-save",
		false,
		"do not write game config JSON",
	)

	return cmd
}

func InstallGame(inputPath string, opts InstallOptions) (*game.Game, engine.Engine, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return nil, nil, errors.New("game path is required")
	}

	resolvedPath, err := filepath.Abs(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve game path: %w", err)
	}

	eng, err := auto.SelectEngine(resolvedPath)
	if err != nil {
		return nil, nil, err
	}

	g, err := eng.InstallGame(resolvedPath)
	if err != nil {
		return nil, nil, fmt.Errorf("install %s game: %w", eng.Name(), err)
	}

	// Depending on your engine implementations, InstallGame may already install
	// the hook. This keeps the command explicit and lets you disable hook install.
	if opts.TextHook {
		if err := eng.InstallTextHook(g); err != nil {
			return nil, nil, fmt.Errorf("install %s text hook: %w", eng.Name(), err)
		}
	}

	if !opts.NoSave {
		output := opts.Output
		if strings.TrimSpace(output) == "" {
			output = DefaultGameConfigPath(g)
		}

		if err := WriteGameConfig(output, g); err != nil {
			return nil, nil, err
		}

		opts.Output = output
	}

	return g, eng, nil
}

func WriteGameConfig(path string, g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("output path is required")
	}

	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal game config: %w", err)
	}
	raw = append(raw, '\n')

	if err := os.WriteFile(resolvedPath, raw, 0o644); err != nil {
		return fmt.Errorf("write game config: %w", err)
	}

	return nil
}

func DefaultGameConfigPath(g *game.Game) string {
	name := strings.TrimSpace(g.Name)
	if name == "" {
		name = "game"
	}

	return filepath.Join(
		configBaseDir(),
		"games",
		util.SanitizeName(name)+".json",
	)
}

func configBaseDir() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "vntext")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".vntext")
	}

	return filepath.Join(home, ".config", "vntext")
}

func init() {
	rootCmd.AddCommand(NewInstallCommand())
}
