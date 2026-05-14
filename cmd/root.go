package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version    = "dev"
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "richmond",
	Short: "Sync Google Workspace users and groups to a SCIM v2 endpoint",
	Long: `Richmond reads users and groups from the Google Workspace Directory API,
maps them to SCIM v2 resources, and pushes creates, updates, and
deactivations to a configurable SCIM v2 endpoint.`,
	Version: version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path")
	rootCmd.SetVersionTemplate(fmt.Sprintf("richmond %s\n", version))
}
