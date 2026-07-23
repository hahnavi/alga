package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"

	"alga/app"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "alga",
	Short: "Alga — webhook alert routing to Mattermost",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		application, err := app.New(cfg)
		if err != nil {
			log.Fatalf("Failed to initialize application: %v", err)
		}
		if err := application.Run(); err != nil {
			log.Fatalf("Application error: %v", err)
		}
	},
}
