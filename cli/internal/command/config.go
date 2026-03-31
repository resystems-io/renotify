package command

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/xdg"
)

func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long: `View and initialise the Renotify configuration.

Subcommands: init, list.`,
		// Skip config loading — these commands must work without
		// a valid settings.json (the user may be running
		// "config init" to create one).
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigListCmd(),
	)

	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var (
		full   bool
		force  bool
		output string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a template settings.json",
		Long: `Create a settings.json configuration file. By default, generates
a minimal file containing only the username field. Use --full to
include all parameters with their compiled defaults.

The file is written to $XDG_CONFIG_HOME/renotify/settings.json
unless --output is specified. Refuses to overwrite an existing
file unless --force is passed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := output
			if path == "" {
				path = xdg.ConfigFilePath()
			}

			// Check for existing file.
			if !force {
				if _, err := os.Stat(path); err == nil {
					return exitcode.Errorf(exitcode.Error,
						"file already exists: %s "+
							"(use --force to overwrite)", path)
				}
			}

			var data []byte
			var err error

			if full {
				data, err = marshalFullConfig()
			} else {
				data, err = marshalMinimalConfig()
			}
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"marshal config: %v", err)
			}

			// Ensure parent directory exists.
			dir := path[:len(path)-len("/settings.json")]
			if len(dir) < len(path) {
				os.MkdirAll(dir, 0700)
			}

			if err := os.WriteFile(path, data, 0600); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"write %s: %v", path, err)
			}

			fmt.Fprintf(cmd.ErrOrStderr(),
				"Config written to %s\n", path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&full, "full", false,
		"include all parameters with defaults")
	cmd.Flags().BoolVar(&force, "force", false,
		"overwrite existing file")
	cmd.Flags().StringVar(&output, "output", "",
		"output path (default: $XDG_CONFIG_HOME/renotify/settings.json)")

	return cmd
}

func newConfigListCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configurable parameters",
		Long: `Print a table of all configurable parameters with their key
path, type, default value, environment variable, and description.

Use --format json for machine-readable output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Default()

			switch format {
			case "json":
				return writeConfigListJSON(cmd, cfg)
			default:
				return writeConfigListText(cmd, cfg)
			}
		},
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}

func writeConfigListText(cmd *cobra.Command, cfg *config.Config) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tTYPE\tDEFAULT\tENV VAR\tDESCRIPTION")
	for _, p := range config.Registry {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			p.Key, p.Type, p.FormatDefault(cfg),
			p.EnvVar, p.Description)
	}
	return w.Flush()
}

type configListEntry struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	EnvVar      string `json:"env_var"`
	Description string `json:"description"`
}

func writeConfigListJSON(cmd *cobra.Command, cfg *config.Config) error {
	entries := make([]configListEntry, len(config.Registry))
	for i, p := range config.Registry {
		entries[i] = configListEntry{
			Key:         p.Key,
			Type:        p.Type,
			Default:     p.FormatDefault(cfg),
			EnvVar:      p.EnvVar,
			Description: p.Description,
		}
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func marshalMinimalConfig() ([]byte, error) {
	cfg := config.Default()
	minimal := map[string]interface{}{
		"username": cfg.Username,
	}
	return json.MarshalIndent(minimal, "", "  ")
}

func marshalFullConfig() ([]byte, error) {
	cfg := config.Default()
	return json.MarshalIndent(cfg, "", "  ")
}
