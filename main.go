package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Version will be set during build.
var Version = "(unknown)"

// CmdMain defines the root command.
var CmdMain = &cobra.Command{
	Use:               "pdf-server [flags]",
	Long:              "...",
	Args:              cobra.NoArgs,
	Version:           Version,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	PersistentPreRunE: setup,
	Run:               runMain,
}

// Initialize command options
func init() {
	// Logging
	CmdMain.Flags().String("log-level", "info", "verbosity of logging output")
	CmdMain.Flags().Bool("log-json", false, "change logging format to JSON")

	// Backend
	CmdMain.Flags().String("listen", ":8080", "address the server should listen to")
}

// runMain is called when the main command is used.
func runMain(_ *cobra.Command, _ []string) {
	// Create routing
	router := chi.NewRouter()

	router.Use(middleware.RedirectSlashes)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.NoCache)
	router.Use(middleware.Recoverer)

	router.Get("/health", healthHandler())
	router.Get("/version", versionHandler())
	router.Post("/convert", convertHandler())

	// Start HTTP server
	srv := &http.Server{
		Addr:    viper.GetString("listen"),
		Handler: router,
	}

	go func() {
		err := srv.ListenAndServe()
		if (err != nil) && (err != http.ErrServerClosed) {
			slog.Error("Failed to start server", slog.Any("error", err))
			os.Exit(1) //nolint:revive
		}
	}()

	slog.Info("Server is listening...", slog.String("address", srv.Addr))

	// Wait for user termination
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-done

	// Stop server
	slog.Info("Server shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Failed to gracefully shut down server", slog.Any("error", err))
		os.Exit(1) //nolint:revive
	}
}

// healthHandler ...
func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render.Status(r, http.StatusOK)
		render.JSON(w, r, map[string]any{"status": "OK"})
	}
}

// versionHandler ...
func versionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render.Status(r, http.StatusOK)
		render.JSON(w, r, Version)
	}
}

// convertHandler ...
func convertHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render.Status(r, http.StatusOK)
		render.JSON(w, r, map[string]any{"foo": "bar"})
	}
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

	err = level.UnmarshalText([]byte(viper.GetString("log-level")))
	if err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}

	var handler slog.Handler

	if viper.GetBool("log-json") {
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
