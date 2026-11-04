package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DarlingGoose/vntext/pkg/app"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/liveclient"
	"github.com/spf13/cobra"
)

// liveFlags are the flags shared by `vntext live …`.
type liveFlags struct {
	Endpoint   string
	API        string
	WS         string
	Token      string
	TokenStdin bool
	Save       bool
	NoInput    bool
}

func (f *liveFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.Endpoint, "endpoint", "", "API preset: local, remote, or an http(s) base URL")
	cmd.Flags().StringVar(&f.API, "api", "", "explicit API base URL (skips endpoint discovery)")
	cmd.Flags().StringVar(&f.WS, "ws", "", "explicit capture websocket URL; env JPN_LEARNING_WS")
	cmd.Flags().StringVar(&f.Token, "token", "", "capture token (kbc_…); env JPN_LEARNING_CAPTURE_TOKEN")
	cmd.Flags().BoolVar(&f.TokenStdin, "token-stdin", false, "read the capture token from stdin")
	cmd.Flags().BoolVar(&f.Save, "save", false, "persist the OAuth client secret in live.json")
	cmd.Flags().BoolVar(&f.NoInput, "no-input", false, "never prompt; fail if credentials are missing")
}

func NewLiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "live",
		Short: "Publish game text to a JPN Learning live capture session",
		Long: `Publish the text vntext already extracts to a live capture session.

The first time you use it, either set the same OAuth client credentials the API
tooling uses (JPN_LEARNING_URL, JPN_LEARNING_CLIENT_ID,
JPN_LEARNING_CLIENT_SECRET), or paste a capture token created on the Live page.
The token is cached in the vntext config directory.`,
	}
	cmd.AddCommand(newLiveLoginCommand())
	cmd.AddCommand(newLiveStatusCommand())
	cmd.AddCommand(newLiveTokenCommand())
	cmd.AddCommand(newLivePipeCommand())
	cmd.AddCommand(newLiveDoctorCommand())
	return cmd
}

func init() {
	rootCmd.AddCommand(NewLiveCommand())
}

// ---- config plumbing ------------------------------------------------------

func liveConfigPath() string {
	return filepath.Join(configBaseDir(), "live.json")
}

func liveResumePath() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = configBaseDir()
	}
	return filepath.Join(cache, app.Name(), "live", "sessions.json")
}

func resolveLiveConfig(flags liveFlags) (liveclient.Config, string, error) {
	path := liveConfigPath()
	cfg, err := liveclient.LoadConfig(path)
	if err != nil {
		return cfg, path, err
	}
	cfg = cfg.ApplyEnv(os.Getenv)

	if strings.TrimSpace(flags.Endpoint) != "" {
		cfg.Endpoint = strings.TrimSpace(flags.Endpoint)
		// The endpoint flag wins over a saved or environment token URL, and
		// over a saved socket: switching deployments switches all of them.
		cfg.APIURL = ""
		cfg.TokenURL = ""
		cfg.WSURL = ""
	}
	if strings.TrimSpace(flags.API) != "" {
		cfg.APIURL = strings.TrimSpace(flags.API)
		cfg.TokenURL = ""
	}
	if strings.TrimSpace(flags.WS) != "" {
		cfg.WSURL = strings.TrimSpace(flags.WS)
	}
	if strings.TrimSpace(flags.Token) != "" {
		cfg.CaptureToken = strings.TrimSpace(flags.Token)
	}
	if flags.TokenStdin {
		token, err := readTokenLine(os.Stdin)
		if err != nil {
			return cfg, path, err
		}
		cfg.CaptureToken = token
	}
	return cfg, path, nil
}

func newLiveClient(cfg liveclient.Config, path string) (*liveclient.Client, *liveclient.TokenSource) {
	client := liveclient.NewClient(cfg)
	source := liveclient.NewTokenSource(client, func(updated liveclient.Config) error {
		// Minting a token should not quietly persist an env-provided OAuth
		// secret; that requires `live login --save`.
		if strings.TrimSpace(os.Getenv("JPN_LEARNING_CLIENT_SECRET")) != "" {
			updated.ClientSecret = ""
		}
		return updated.Save(path)
	})
	return client, source
}

// ensureLiveToken returns a usable capture token, minting one from the OAuth
// client credentials or asking for a pasted token. It never runs in non-TTY
// contexts unless a token is already configured.
func ensureLiveToken(ctx context.Context, client *liveclient.Client, source *liveclient.TokenSource, flags liveFlags, out io.Writer) (string, error) {
	cfg := client.Config()
	if cfg.CaptureToken != "" {
		source.Set(cfg.CaptureToken)
		return cfg.CaptureToken, nil
	}
	if cfg.ClientID != "" && cfg.ClientSecret != "" {
		token, err := source.Token(ctx)
		if err != nil {
			return "", err
		}
		return token, nil
	}
	if flags.NoInput {
		return "", fmt.Errorf("%w: run `%s live login`, set JPN_LEARNING_CLIENT_ID and JPN_LEARNING_CLIENT_SECRET, or pass --token", liveclient.ErrNotConfigured, app.Name())
	}
	token, err := promptCaptureToken(cfg, out)
	if err != nil {
		return "", err
	}
	source.Set(token)
	cfg.CaptureToken = token
	client.SetConfig(cfg)
	return token, nil
}

func promptCaptureToken(cfg liveclient.Config, out io.Writer) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("%w: no terminal available; pass --token or set JPN_LEARNING_CLIENT_SECRET", liveclient.ErrNotConfigured)
	}
	defer tty.Close()

	fmt.Fprintln(tty, "Live capture needs a capture token.")
	fmt.Fprintf(tty, "  1. Open %s → Capture tokens → Create\n", tokenPageURL(cfg))
	fmt.Fprint(tty, "  2. Paste the secret (kbc_…) here: ")

	reader := bufio.NewReader(tty)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read token: %w", err)
	}
	token := strings.TrimSpace(line)
	if token == "" {
		return "", errors.New("no token entered")
	}
	if !strings.HasPrefix(token, "kbc_") {
		return "", errors.New("that does not look like a capture token (it should start with kbc_)")
	}
	if out != nil {
		fmt.Fprintln(out, "token received")
	}
	return token, nil
}

func tokenPageURL(cfg liveclient.Config) string {
	switch strings.ToLower(strings.TrimSpace(cfg.Endpoint)) {
	case "local":
		return "http://127.0.0.1:3010/live"
	case "", "remote":
		return "https://jpn-learning.darlinggoose.com/live"
	default:
		if base, err := cfg.BaseURL(); err == nil {
			return strings.TrimRight(base, "/") + "/live"
		}
		return "the JPN Learning Live page"
	}
}

func readTokenLine(reader io.Reader) (string, error) {
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read token: %w", err)
		}
		return "", errors.New("no token on stdin")
	}
	token := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(token, "kbc_") {
		return "", errors.New("stdin did not contain a kbc_ capture token")
	}
	return token, nil
}

// ---- login ----------------------------------------------------------------

func newLiveLoginCommand() *cobra.Command {
	var flags liveFlags
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Mint and cache a capture token from the configured OAuth client",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, path, err := resolveLiveConfig(flags)
			if err != nil {
				return err
			}
			client, source := newLiveClient(cfg, path)

			endpoints, err := client.Endpoints(ctx)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "endpoint   %s\n", endpoints.APIBase)
			fmt.Fprintf(out, "token url  %s\n", endpoints.TokenURL)
			fmt.Fprintf(out, "websocket  %s\n", endpoints.CaptureWS)

			// Persist an env-provided secret only when the user asked for it.
			persist := func(current liveclient.Config) liveclient.Config {
				if !flags.Save && strings.TrimSpace(os.Getenv("JPN_LEARNING_CLIENT_SECRET")) != "" {
					current.ClientSecret = ""
				}
				return current
			}

			hasCreds := cfg.ClientID != "" && cfg.ClientSecret != ""
			var accessToken string
			needMint := cfg.CaptureToken == ""

			if !needMint {
				probeErr := client.ProbeHandshake(ctx, cfg.CaptureToken)
				switch {
				case probeErr == nil:
					if err := persist(cfg).Save(path); err != nil {
						return err
					}
					fmt.Fprintf(out, "capture    %s (verified, saved to %s)\n", redactedToken(cfg.CaptureToken), path)
				case errors.Is(probeErr, liveclient.ErrUnauthorized) && hasCreds:
					fmt.Fprintf(out, "capture    cached token rejected (%v); minting a new one\n", probeErr)
					cfg.CaptureToken = ""
					cfg.CaptureTokenID = ""
					client.SetConfig(cfg)
					needMint = true
				case errors.Is(probeErr, liveclient.ErrUnauthorized):
					return probeErr
				default:
					// The token may be fine and the socket broken. Do not burn
					// a working credential over a reachability problem.
					fmt.Fprintf(out, "capture    %s kept, but the socket check failed: %v\n", redactedToken(cfg.CaptureToken), probeErr)
					if err := persist(cfg).Save(path); err != nil {
						return err
					}
				}
			}

			if needMint {
				accessToken, err = client.ExchangeClientCredentials(ctx)
				if err != nil {
					return err
				}
				token, secret, err := client.MintCaptureToken(ctx, accessToken)
				if err != nil {
					return err
				}
				cfg = client.Config()
				cfg.APIURL = endpoints.APIBase
				cfg.CaptureTokenID = token.ID
				if err := persist(cfg).Save(path); err != nil {
					return err
				}
				source.Set(secret)
				fmt.Fprintf(out, "capture    %s  saved to %s\n", redactedToken(secret), path)

				if err := client.ProbeHandshake(ctx, secret); err != nil {
					fmt.Fprintf(out, "warning    the new token could not connect: %v\n", err)
				}
			}

			if accessToken == "" && client.Config().ClientID != "" && client.Config().ClientSecret != "" {
				accessToken, _ = client.ExchangeClientCredentials(ctx)
			}
			if accessToken != "" {
				if status, err := client.Status(ctx, accessToken); err != nil {
					fmt.Fprintf(out, "account    token works; status unavailable: %v\n", err)
				} else {
					fmt.Fprintf(out, "account    ok · %d active session(s)\n", status.Totals.Sessions)
				}
			}
			return nil
		},
	}
	flags.bind(cmd)
	return cmd
}

// ---- status ---------------------------------------------------------------

func newLiveStatusCommand() *cobra.Command {
	var flags liveFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether a publisher or watchers are connected",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, path, err := resolveLiveConfig(flags)
			if err != nil {
				return err
			}
			client, _ := newLiveClient(cfg, path)
			accessToken, err := client.ExchangeClientCredentials(ctx)
			if err != nil {
				return fmt.Errorf("%w (status needs the OAuth client credentials; capture tokens cannot read presence)", err)
			}
			summary, err := client.Status(ctx, accessToken)
			if err != nil {
				return err
			}
			printStatus(cmd.OutOrStdout(), summary)
			return nil
		},
	}
	flags.bind(cmd)
	return cmd
}

func printStatus(out io.Writer, summary *liveclient.StatusSummary) {
	if summary == nil || len(summary.Sessions) == 0 {
		fmt.Fprintln(out, "no live sessions")
		return
	}
	fmt.Fprintf(out, "%d session(s) · %d publishing · %d watching\n",
		summary.Totals.Sessions, summary.Totals.Publishing, summary.Totals.Watching)
	for _, session := range summary.Sessions {
		publisher := "no publisher"
		if session.Publishing.Connected {
			publisher = "publishing"
			if session.Publishing.DeviceName != "" {
				publisher += " from " + session.Publishing.DeviceName
			}
			if session.Publishing.IdleSeconds > 0 {
				publisher += fmt.Sprintf(" (idle %ds)", session.Publishing.IdleSeconds)
			}
		}
		fmt.Fprintf(out, "  %-24s %-9s %5d lines · %s · %d watching\n",
			truncate(session.GameName, 24), session.Status, session.LineCount, publisher, session.Watching.Count)
	}
}

// ---- token management -----------------------------------------------------

func newLiveTokenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "List or revoke capture tokens",
	}
	cmd.AddCommand(newLiveTokenListCommand())
	cmd.AddCommand(newLiveTokenRevokeCommand())
	return cmd
}

func newLiveTokenListCommand() *cobra.Command {
	var flags liveFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List capture tokens (secrets are not shown)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, path, err := resolveLiveConfig(flags)
			if err != nil {
				return err
			}
			client, _ := newLiveClient(cfg, path)
			accessToken, err := client.ExchangeClientCredentials(ctx)
			if err != nil {
				return err
			}
			tokens, err := client.ListTokens(ctx, accessToken)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(tokens) == 0 {
				fmt.Fprintln(out, "no capture tokens")
				return nil
			}
			for _, token := range tokens {
				state := "active"
				if token.RevokedAt != nil {
					state = "revoked"
				}
				fmt.Fprintf(out, "%s  %-10s %-24s %s", token.ID, token.Prefix, truncate(token.Name, 24), state)
				if token.LastUsedAt != nil {
					fmt.Fprintf(out, " · last used %s", token.LastUsedAt.Local().Format(time.RFC3339))
				}
				fmt.Fprintln(out)
			}
			return nil
		},
	}
	flags.bind(cmd)
	return cmd
}

func newLiveTokenRevokeCommand() *cobra.Command {
	var flags liveFlags
	var id string
	cmd := &cobra.Command{
		Use:   "revoke [id]",
		Short: "Revoke a capture token",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, path, err := resolveLiveConfig(flags)
			if err != nil {
				return err
			}
			client, _ := newLiveClient(cfg, path)
			tokenID := strings.TrimSpace(id)
			if len(args) > 0 {
				tokenID = strings.TrimSpace(args[0])
			}
			if tokenID == "" {
				tokenID = cfg.CaptureTokenID
			}
			if tokenID == "" {
				return errors.New("which token? pass its id, or use `live token list`")
			}
			accessToken, err := client.ExchangeClientCredentials(ctx)
			if err != nil {
				return err
			}
			if err := client.RevokeToken(ctx, accessToken, tokenID); err != nil {
				return err
			}
			if tokenID == cfg.CaptureTokenID {
				cfg.CaptureToken = ""
				cfg.CaptureTokenID = ""
				_ = cfg.Save(path)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "revoked %s\n", tokenID)
			return nil
		},
	}
	flags.bind(cmd)
	cmd.Flags().StringVar(&id, "id", "", "token id")
	return cmd
}

// ---- pipe -----------------------------------------------------------------

func newLivePipeCommand() *cobra.Command {
	var flags liveFlags
	var gameName, device, speaker, kind string
	var noAutoSave, dryRun bool

	cmd := &cobra.Command{
		Use:   "pipe",
		Short: "Publish newline-delimited text from stdin",
		Long: `Read newline-delimited UTF-8 from stdin and publish each line to a live
session. This is the quickest way to test a hook or to feed another extractor
without launching a game through vntext.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(gameName) == "" {
				return errors.New("--game is required: it names the project the lines are saved under")
			}
			if flags.TokenStdin {
				return errors.New("--token-stdin cannot be used with pipe: stdin carries the lines; use --token or the cached token")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			cfg, path, err := resolveLiveConfig(flags)
			if err != nil {
				return err
			}
			if kind == "" {
				kind = cfg.DefaultKind
			}
			client, source := newLiveClient(cfg, path)
			if !dryRun {
				if _, err := ensureLiveToken(ctx, client, source, flags, cmd.OutOrStdout()); err != nil {
					return err
				}
			}

			publisher, err := liveclient.NewPublisher(ctx, client, source, liveclient.PublisherOptions{
				GameName:   gameName,
				DeviceName: device,
				Kind:       kind,
				AutoSave:   !noAutoSave && client.Config().AutoSaveEnabled(),
				ResumeFile: liveResumePath(),
				DryRun:     dryRun,
				Out:        cmd.OutOrStdout(),
			})
			if err != nil {
				return err
			}

			lines := make(chan string)
			go func() {
				defer close(lines)
				scanner := bufio.NewScanner(cmd.InOrStdin())
				scanner.Buffer(make([]byte, 64*1024), 1024*1024)
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line == "" {
						continue
					}
					select {
					case lines <- line:
					case <-ctx.Done():
						return
					}
				}
				if err := scanner.Err(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "read stdin: %v\n", err)
				}
			}()

		feed:
			for {
				select {
				case <-ctx.Done():
					break feed
				case line, ok := <-lines:
					if !ok {
						break feed
					}
					publisher.Offer(liveclient.Line{Text: line, Speaker: speaker, Kind: kind, At: time.Now()})
				}
			}

			closeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := publisher.Close(closeCtx); err != nil {
				return err
			}
			stats := publisher.Stats()
			fmt.Fprintf(cmd.ErrOrStderr(), "live: %d sent, %d acknowledged, %d dropped\n", stats.Sent, stats.Acked, stats.Dropped)
			return nil
		},
	}
	flags.bind(cmd)
	cmd.Flags().StringVar(&gameName, "game", "", "game name; the project is created from it")
	cmd.Flags().StringVar(&device, "device", "", "device label shown on the live page")
	cmd.Flags().StringVar(&speaker, "speaker", "", "default speaker for every line")
	cmd.Flags().StringVar(&kind, "kind", "", "default line kind: dialogue, narration, choice or system")
	cmd.Flags().BoolVar(&noAutoSave, "no-auto-save", false, "stream lines but do not save them as project entries")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print frames instead of connecting")
	return cmd
}

// ---- doctor ---------------------------------------------------------------

func newLiveDoctorCommand() *cobra.Command {
	var flags liveFlags
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Show the resolved configuration and test each credential",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, path, err := resolveLiveConfig(flags)
			if err != nil {
				return err
			}
			client, _ := newLiveClient(cfg, path)
			out := cmd.OutOrStdout()

			fmt.Fprintf(out, "config     %s\n", path)
			fmt.Fprintf(out, "endpoint   %s\n", cfg.Endpoint)
			fmt.Fprintf(out, "oauth      client_id=%t client_secret=%t\n", cfg.ClientID != "", cfg.ClientSecret != "")
			fmt.Fprintf(out, "capture    %s\n", redactedToken(cfg.CaptureToken))

			endpoints, err := client.Endpoints(ctx)
			if err != nil {
				fmt.Fprintf(out, "api        error: %v\n", err)
				return nil
			}
			fmt.Fprintf(out, "api base   %s\n", endpoints.APIBase)
			fmt.Fprintf(out, "token url  %s\n", endpoints.TokenURL)
			fmt.Fprintf(out, "websocket  %s\n", endpoints.CaptureWS)

			if token := client.Config().CaptureToken; token != "" {
				if err := client.ProbeHandshake(ctx, token); err != nil {
					fmt.Fprintf(out, "socket     failed: %v\n", err)
				} else {
					fmt.Fprintf(out, "socket     ok (%s answered hello)\n", redactedToken(token))
				}
			} else if cfg.ClientID != "" && cfg.ClientSecret != "" {
				if _, err := client.ExchangeClientCredentials(ctx); err != nil {
					fmt.Fprintf(out, "oauth      could not exchange credentials: %v\n", err)
				} else {
					fmt.Fprintln(out, "oauth      credentials accepted (run `vntext live login` to mint a token)")
				}
			} else {
				fmt.Fprintln(out, "token      missing; run `vntext live login`")
			}
			return nil
		},
	}
	flags.bind(cmd)
	return cmd
}

// ---- run-game wiring ------------------------------------------------------

// prepareLiveCapture resolves the endpoint and credentials before the game is
// launched. It deliberately does not connect: the first line still creates the
// session, so an idle launch does not leave an empty project section.
func prepareLiveCapture(ctx context.Context, opts RunGameOptions, out io.Writer) error {
	flags := liveFlags{
		Endpoint: opts.Live.Endpoint,
		API:      opts.Live.API,
		WS:       opts.Live.WS,
		Token:    opts.Live.Token,
		NoInput:  opts.Live.NoInput,
	}
	cfg, path, err := resolveLiveConfig(flags)
	if err != nil {
		return err
	}
	client, source := newLiveClient(cfg, path)
	if cfg.CaptureToken != "" {
		// A token from the flag, the environment or the file is already
		// usable; do not copy it into the config file as a side effect.
		return nil
	}
	if _, err := ensureLiveToken(ctx, client, source, flags, out); err != nil {
		return err
	}
	updated := client.Config()
	if strings.TrimSpace(os.Getenv("JPN_LEARNING_CLIENT_SECRET")) != "" {
		updated.ClientSecret = ""
	}
	return updated.Save(path)
}

func newLivePublisher(ctx context.Context, g *game.Game, opts RunGameOptions, out io.Writer) (*liveclient.Publisher, error) {
	if !opts.Live.Enabled {
		return nil, nil
	}
	if out == nil {
		out = io.Discard
	}
	flags := liveFlags{
		Endpoint: opts.Live.Endpoint,
		API:      opts.Live.API,
		WS:       opts.Live.WS,
		Token:    opts.Live.Token,
		NoInput:  opts.Live.NoInput,
	}
	cfg, path, err := resolveLiveConfig(flags)
	if err != nil {
		return nil, err
	}
	client, source := newLiveClient(cfg, path)

	if !opts.Live.DryRun {
		if _, err := ensureLiveToken(ctx, client, source, flags, out); err != nil {
			return nil, err
		}
	}
	kind := strings.TrimSpace(opts.Live.Kind)
	if kind == "" {
		kind = client.Config().DefaultKind
	}
	gameName := strings.TrimSpace(opts.Live.GameName)
	if gameName == "" {
		gameName = g.Name
	}
	return liveclient.NewPublisher(ctx, client, source, liveclient.PublisherOptions{
		GameName:   gameName,
		DeviceName: opts.Live.Device,
		Kind:       kind,
		AutoSave:   !opts.Live.NoAutoSave && client.Config().AutoSaveEnabled(),
		ResumeFile: liveResumePath(),
		DryRun:     opts.Live.DryRun,
		Out:        out,
	})
}

func redactedToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return "(none)"
	}
	if len(token) <= 12 {
		return token
	}
	return token[:12] + "…"
}

func truncate(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "…"
}
