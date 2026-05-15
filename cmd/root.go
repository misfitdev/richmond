package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version    = "dev"
	commit     = "unknown"
	configFile string
	logLevel   string
)

var rootCmd = &cobra.Command{
	Use:   "richmond",
	Short: "Sync Google Workspace users and groups to a SCIM v2 endpoint",
	Long: `Richmond reads users and groups from the Google Workspace Directory API,
maps them to SCIM v2 resources, and pushes creates, updates, and
deactivations to a configurable SCIM v2 endpoint.`,
	Version: version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		setupLogging(cmd, args)
		slog.Info("richmond starting", "version", version, "commit", commit)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.SilenceUsage = true
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.SetVersionTemplate(fmt.Sprintf("richmond %s (%s)\n", version, commit))
}

func setupLogging(_ *cobra.Command, _ []string) {
	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}
