package cmd

import (
	"fmt"

	"github.com/guettli/watchall/internal/deltas"
	"github.com/spf13/cobra"
)

var deltasCmd = &cobra.Command{
	Use:   "deltas dir",
	Short: "show the deltas (changes) of resource objects",
	Long:  `This reads the files from the local disk and shows the changes. No connection to a cluster is needed.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("At least one argument (directory of yaml files, created by `record`) is needed")
		}
		dir := args[0]
		return deltas.Deltas(dir)
	},
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(deltasCmd)
}
