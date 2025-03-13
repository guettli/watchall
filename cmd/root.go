package cmd

import (
	"os"

	"github.com/guettli/watchall/record"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "watchall",
	Short: "Watch resources in your Kubernetes cluster.",
	Long:  `...`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var arguments = record.Arguments{}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	RootCmd.PersistentFlags().BoolVarP(&arguments.Verbose, "verbose", "v", false, "Create more output")
	RootCmd.PersistentFlags().StringVarP(&arguments.OutputDirectory, "outdir", "o", "watchall-output", "Directory to store output")
}
