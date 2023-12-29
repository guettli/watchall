package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/guettli/watchall/config"
	"github.com/guettli/watchall/record"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var rootCmd = &cobra.Command{
	Use:   "run",
	Short: "pull Kubernetes resources into local DB and run web UI",
	Long:  `...`,
	Run: func(cmd *cobra.Command, args []string) {
		runArgs(arguments)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func runArgs(args config.Arguments) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	wg, err := record.RunRecordWithContext(context.Background(), args, kubeconfig)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	wg.Wait()
}

var arguments = config.Arguments{}

func init() {
	// Here you will define your flags and configuration settings.

	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().BoolVarP(&arguments.Verbose, "verbose", "v", false, "Create more output")
}
