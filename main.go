package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// CmdMain defines the root command.
var CmdMain = &cobra.Command{
	Use:               "pdf-server [flags]",
	Long:              "...",
	Args:              cobra.NoArgs,
	Version:           "0.0.1",
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	PersistentPreRunE: setup,
}

// Initialize command options
func init() {
	// Logging
	CmdMain.Flags().String("logging.level", "info", "verbosity of logging output")
	CmdMain.Flags().Bool("logging.json", false, "change logging format to JSON")
}

// setup will set up configuration management and logging.
//
// Configuration options can be set via the command line, via a configuration file (in the current folder, at
// "/etc/pdf-server/config.yaml" or at "~/.config/pdf-server/config.yaml"), and via environment variables (all
// uppercase and prefixed with "PDF_SERVER_").
func setup(cmd *cobra.Command, _ []string) error {
	// Connect all options to Viper
	err := viper.BindPFlags(cmd.Flags())
	if err != nil {
		return fmt.Errorf("bind command line flags: %w", err)
	}

	// Environment variables
	viper.SetEnvPrefix("PDF_SERVER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// Configuration file
	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/pdf-server")
	viper.AddConfigPath("$HOME/.config/pdf-server")
	viper.AddConfigPath(".")

	viper.ReadInConfig() //nolint:errcheck

	// Logging
	var level slog.Level

	err = level.UnmarshalText([]byte(viper.GetString("logging.level")))
	if err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}

	var handler slog.Handler

	if viper.GetBool("logging.json") {
		// Use JSON handler
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	} else {
		// Use text handler
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}

	slog.SetDefault(slog.New(handler))

	return nil
}

// main is the main entry point of the command.
func main() {
	if err := CmdMain.Execute(); err != nil {
		slog.Error("Unable to execute command", slog.Any("error", err))
	}
}
