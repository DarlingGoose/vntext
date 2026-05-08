package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/gameConfig"
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

			g, eng, err := gameConfig.InstallGame(cmd.Context(), args[0], opts.TextHook, opts.Output)
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
