package main

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/gographics/imagick.v2/imagick"
)

// Version will be set during build.
var Version = "(unknown)"

// CmdMain defines the root command.
var CmdMain = &cobra.Command{
	Use:               "magick-server [flags]",
	Long:              "A simple API server to convert (multi-page) images using ImageMagick.",
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
	CmdMain.Flags().String("listen", ":8081", "address the server should listen to")
}

// runMain is called when the main command is used.
func runMain(_ *cobra.Command, _ []string) {
	// Initialization ImageMagick
	imagick.Initialize()
	defer imagick.Terminate()

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

// setup will set up configuration management and logging.
//
// Configuration options can be set via the command line, via a configuration file (in the current folder, at
// "/etc/magck-server/config.yaml" or at "~/.config/magick-server/config.yaml"), and via environment variables
// (all uppercase and prefixed with "MAGICK_SERVER_").
func setup(cmd *cobra.Command, _ []string) error {
	// Connect all options to Viper
	err := viper.BindPFlags(cmd.Flags())
	if err != nil {
		return fmt.Errorf("bind command line flags: %w", err)
	}

	// Environment variables
	viper.SetEnvPrefix("MAGICK_SERVER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// Configuration file
	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/magick-server")
	viper.AddConfigPath("$HOME/.config/magick-server")
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
		slog.Error("Failed to execute command", slog.Any("error", err))
	}
}

// healthHandler returns the health status.
func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Return JSON with health
		render.Status(r, http.StatusOK)
		render.JSON(w, r, map[string]any{"status": "OK"})
	}
}

// versionHandler returns the server version.
func versionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Return JSON with version
		render.Status(r, http.StatusOK)
		render.JSON(w, r, Version)
	}
}

// formatExtensionMap defines the supported output formats and their file extensions.
var formatExtensionMap = map[string]string{
	"JPEG": "jpg",  // JPEG File Interchange Format
	"PNG":  "png",  // Portable Network Graphics
	"TIFF": "tiff", // Tagged Image File Format
}

// layoutType defines the output layout to enforce.
type layoutType string

const (
	layoutTypeLandscape layoutType = "LANDSCAPE" // layoutTypeLandscape forces a landscape layout.
	layoutTypePortrait  layoutType = "PORTRAIT"  // layoutTypePortrait forces a portrait layout.
	layoutTypeKeep      layoutType = "KEEP"      // layoutTypeKeep keeps the original layout.
)

// convertHandler converts a (multi-page) image into a Zip archive.
func convertHandler() http.HandlerFunc { //nolint
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse density
		density := 300.0

		if v := r.URL.Query().Get("density"); v != "" {
			d, err := strconv.ParseFloat(v, 64)
			if err != nil {
				slog.Error("Failed to parse validate density", slog.Any("error", err), slog.String("value", v))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, map[string]any{"error": "invalid density"})
				return
			}

			density = d
		}

		// Parse compression quality
		quality := uint(85)

		if v := r.URL.Query().Get("quality"); v != "" {
			q, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				slog.Error("Failed to parse compression quality", slog.Any("error", err), slog.String("value", v))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, map[string]any{"error": "invalid compression quality"})
				return
			}

			quality = uint(q)
		}

		// Parse output format
		format := "JPEG"

		if v := r.URL.Query().Get("format"); v != "" {
			v = strings.ToUpper(v)
			if _, ok := formatExtensionMap[v]; !ok {
				slog.Error("Failed to parse output format", slog.String("value", v))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, map[string]any{"error": "invalid output format"})
				return
			}

			format = v
		}

		// Parse output layout
		layout := layoutTypeKeep

		if v := r.URL.Query().Get("layout"); v != "" {
			v = strings.ToUpper(v)
			if (v != string(layoutTypeLandscape)) && (v != string(layoutTypePortrait)) && (v != string(layoutTypeKeep)) {
				slog.Error("Failed to parse output layout", slog.String("value", v))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, map[string]any{"error": "invalid output layout"})
				return
			}

			layout = layoutType(v)
		}

		// Read request body
		in, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("Failed to read request body", slog.Any("error", err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, map[string]any{"error": "failed to read request body"})
			return
		}

		// Get a new magick wand
		mw := imagick.NewMagickWand()
		defer mw.Destroy()

		// Set density
		err = mw.SetResolution(density, density)
		if err != nil {
			slog.Error("Failed to set density", slog.Any("error", err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, map[string]any{"error": "failed to set density"})
			return
		}

		// Read image
		err = mw.ReadImageBlob(in)
		if err != nil {
			slog.Error("Failed to read image", slog.Any("error", err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, map[string]any{"error": "failed to read image"})
			return
		}

		// Set up Zip archive
		buf := &bytes.Buffer{}
		zipWriter := zip.NewWriter(buf)

		zipWriter.RegisterCompressor(zip.Deflate, func(o io.Writer) (io.WriteCloser, error) {
			return flate.NewWriter(o, flate.BestSpeed)
		})

		// Iterate through all pages
		mw.ResetIterator()

		for page := 0; mw.NextImage(); page++ {
			// Pull current image into its own magick wand
			mwi := mw.GetImage()
			defer mwi.Destroy()

			// Flatten image
			mwm := mwi.MergeImageLayers(imagick.IMAGE_LAYER_FLATTEN)
			defer mwm.Destroy()

			// Set compression quality
			err = mwm.SetImageCompressionQuality(quality)
			if err != nil {
				slog.Error("Failed to set compression quality", slog.Any("error", err), slog.Any("quality", quality))
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, map[string]any{"error": "failed to set compression quality"})
				return
			}

			// Set output format
			err = mwm.SetImageFormat(format)
			if err != nil {
				slog.Error("Failed to set output format", slog.Any("error", err), slog.String("format", format))
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, map[string]any{"error": "failed to set output format"})
				return
			}

			// Force output layout
			switch layout {
			case layoutTypeLandscape:
				// Get dimensions
				width := mwm.GetImageWidth()
				height := mwm.GetImageHeight()

				if width < height {
					// Rotate image
					err := mwm.RotateImage(imagick.NewPixelWand(), -90.0)
					if err != nil {
						slog.Error("Failed to rotate image", slog.Any("error", err))
						render.Status(r, http.StatusInternalServerError)
						render.JSON(w, r, map[string]any{"error": "failed to rotate image"})
						return
					}
				}

			case layoutTypePortrait:
				// Get dimensions
				width := mwm.GetImageWidth()
				height := mwm.GetImageHeight()

				if height < width {
					// Rotate image
					err := mwm.RotateImage(imagick.NewPixelWand(), -90.0)
					if err != nil {
						slog.Error("Failed to rotate image", slog.Any("error", err))
						render.Status(r, http.StatusInternalServerError)
						render.JSON(w, r, map[string]any{"error": "failed to rotate image"})
						return
					}
				}

			case layoutTypeKeep:
				// Do nothing
			}

			// Get output blob
			out, err := mwm.GetImageBlob()
			if err != nil {
				slog.Error("Failed to get output blob", slog.Any("error", err))
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, map[string]any{"error": "failed to set output format"})
				return
			}

			// Create new Zip archive entry
			f, err := zipWriter.Create(fmt.Sprintf("%04d.%s", page, formatExtensionMap[format]))
			if err != nil {
				slog.Error("Failed to create new Zip archive entry", slog.Any("error", err))
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, map[string]any{"error": "failed to create new Zip archive entry"})
				return
			}

			// Write image into Zip archive
			_, err = f.Write(out)
			if err != nil {
				slog.Error("Failed to write image into Zip archive", slog.Any("error", err))
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, map[string]any{"error": "failed to write image into Zip archive"})
				return
			}
		}

		// Close Zip archive
		err = zipWriter.Close()
		if err != nil {
			slog.Error("Failed to close Zip archive", slog.Any("error", err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, map[string]any{"error": "failed to close Zip archive"})
			return
		}

		// We're good
		render.Status(r, http.StatusOK)
		render.Data(w, r, buf.Bytes())
	}
}
