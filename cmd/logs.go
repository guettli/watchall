package cmd

import (
	"context"
	"io"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Check all logs of all pods",
	Long:  `...`,
	Run: func(cmd *cobra.Command, args []string) {
		runLogs(arguments)
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
}

func runLogs(args Arguments) {
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
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
				Container: container.Name,
			})

			podLogs, err := req.Stream(context.TODO())
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
