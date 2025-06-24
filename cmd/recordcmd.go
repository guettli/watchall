package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

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
	recordCmd.Flags().BoolVarP(&arguments.WithLogs, "with-logs", "w", false, "Record logs of pods")
	recordCmd.Flags().StringVar(&arguments.IgnoreLogLinesFile, "ignore-log-lines-file", "", "Path to file containing log lines to ignore. Syntax of line based file format: 'filename-regex ~~ line-regex'. If line-regex is empty the pod won't be watched. Lines starting with '#', and empty lines get ignored. Example to ignore info lines of cilium: kube-system/cilium ~~ level=info. Alternatively you can use --skip when using the 'deltas' sub-command.")
	recordCmd.Flags().BoolVarP(&arguments.DisableResourceRecording, "disable-resource-recording", "", false, "Do not watch/record changes to resources. Only meaningful if you only want logs: --with-logs.")
	RootCmd.AddCommand(recordCmd)
}

func runRecord(args record.Arguments) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	if args.DisableResourceRecording && !args.WithLogs {
		fmt.Println("Error: --skip-recording-resources is only meaningful with --with-logs")
		os.Exit(1)
	}
	if args.IgnoreLogLinesFile != "" {
		err := parseIgnoreLogLinesFile(args.IgnoreLogLinesFile, &args)
		if err != nil {
			fmt.Printf("Error parsing ignore-log-lines-file %q: %v\n", args.IgnoreLogLinesFile, err)
			os.Exit(1)
		}
	}

	wg, err := record.RunRecordWithContext(context.Background(), args, kubeconfig)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	wg.Wait()
}

func parseIgnoreLogLinesFile(filename string, args *record.Arguments) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return err
	}
	lines, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("Error reading file: %w", err)
	}
	for _, line := range strings.Split(string(lines), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}
		parts := strings.Split(line, "~~")
		if len(parts) > 2 {
			return fmt.Errorf("Invalid line. Expected file-regex ~~ line-regex: %q\n", line)
		}
		fileRegex := strings.TrimSpace(parts[0])
		if fileRegex == "" {
			return fmt.Errorf("File regex is empty: %q\n", line)
		}
		fRegex, err := regexp.Compile(fileRegex)
		if err != nil {
			return fmt.Errorf("Invalid file regex %q: %w", fileRegex, err)
		}

		var lineRegex string
		if len(parts) == 2 {
			lineRegex = strings.TrimSpace(parts[1])
		}

		if lineRegex == "" {
			args.IgnorePods = append(args.IgnorePods, fRegex)
			continue
		}
		lRegex, err := regexp.Compile(lineRegex)
		if err != nil {
			return fmt.Errorf("Invalid line regex %q: %w", lineRegex, err)
		}
		args.IgnoreLogLines = append(args.IgnoreLogLines, record.IgnoreLogLine{
			FileRegex: fRegex,
			LineRegex: lRegex,
		})
	}
	return nil
}
