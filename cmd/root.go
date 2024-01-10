package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/guettli/watchall/config"
	"github.com/guettli/watchall/dbstuff"
	"github.com/guettli/watchall/record"
	"github.com/guettli/watchall/ui"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"

	_ "net/http/pprof"
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

var errSIGINT = fmt.Errorf("received SIGINT (ctrl-c)")

func runArgs(args config.Arguments) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeconfig.ClientConfig()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	db, host, err := dbstuff.GetDB(config.Host)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	defer func() {
		db.Close()
		fmt.Println("db was closed.")
	}()

	args.Db = db
	args.FatalErrorChannel = make(chan error)
	args.StoreChannel = make(chan *unstructured.Unstructured)

	ctx, cancelFunc := context.WithCancelCause(context.Background())
	args.CancelFunc = cancelFunc

	go func() {
		for {
			// Somehow the first ctrl-c is sometimes not enough.
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT) // catch ctrl-c
			sig := <-sigs
			fmt.Printf("Received signal %+v\n", sig)
			args.FatalErrorChannel <- errSIGINT

			time.Sleep(3 * time.Second)

			// The program has not terminated yet? Sad, that's not what we want.
			fmt.Println("Not terminated yet? Let's have a look at the goroutines")
			pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)

		}
	}()

	go func() {
		handleFatalErrorChannel(ctx, &args)
	}()

	fmt.Println("per HandleStoreChannel")
	go func() {
		record.HandleStoreChannel(ctx, &args)
	}()

	fmt.Println("per RunRecordWithContext")

	if true {
		err := record.RunRecordWithContext(ctx, args, config, host)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
	}
	fmt.Println("per RunUIWithContext")

	go func() {
		ui.RunUIWithContext(ctx, args, db)
	}()

	<-ctx.Done()
	fmt.Println("post waitttttttttttt")
}

var arguments = config.Arguments{}

func handleFatalErrorChannel(ctx context.Context, args *config.Arguments) {
	select {
	case err := <-args.FatalErrorChannel:
		if !errors.Is(err, errSIGINT) {
			fmt.Printf("handleFatalErrorChannel received: %+v\n", err)
		}
		args.CancelFunc(err)
		return
	case <-ctx.Done():
		return
	}
}

func init() {
	// Here you will define your flags and configuration settings.

	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().BoolVarP(&arguments.Verbose, "verbose", "v", false, "Create more output")
}
