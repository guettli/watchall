package cmd

import (
	"github.com/guettli/watchall/internal/deltas"
	"github.com/spf13/cobra"
)

var deltasCmd = &cobra.Command{
	Use:   "deltas dir",
	Short: "show the deltas (changes) of resource objects",
	Long:  `This reads the files from the local disk and shows the changes. No connection to a cluster is needed.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := args[0]
		return deltas.Deltas(dir, skipPatterns)
	},
	SilenceUsage: true,
}

var skipPatterns []string

func init() {
	rootCmd.AddCommand(deltasCmd)
	deltasCmd.Flags().StringSliceVar(&skipPatterns, "skip", []string{}, "comma separated list of regex patterns to skip")
}
