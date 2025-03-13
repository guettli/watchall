package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/guettli/watchall/record"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "record all changes to resource objects",
	Long:  `...`,
	Run: func(_ *cobra.Command, _ []string) {
		runRecord(arguments)
	},
}

func init() {
	RootCmd.AddCommand(recordCmd)
}

func runRecord(args record.Arguments) {
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
