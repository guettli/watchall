package cmd

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Check all logs of all pods",
	Long:  `...`,
	Run: func(_ *cobra.Command, _ []string) {
		runLogs()
	},
}

func init() {
	RootCmd.AddCommand(logsCmd)
}

func runLogs() {
	ctx := context.Background()
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeconfig.ClientConfig()
	if err != nil {
		panic(err.Error())
	}

	config.QPS = 1000
	config.Burst = 1000

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// List all pods
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
				Container: container.Name,
			})

			podLogs, err := req.Stream(ctx)
			if err != nil {
				panic(err.Error())
			}
			defer podLogs.Close()

			_, err = io.Copy(os.Stdout, podLogs)
			if err != nil {
				panic(err.Error())
			}
		}
	}
}
