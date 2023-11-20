package cmd

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"

	"sigs.k8s.io/yaml"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "record all changes to resource objects",
	Long:  `...`,
	Run: func(cmd *cobra.Command, args []string) {
		runRecord(arguments)
	},
}

func init() {
	rootCmd.AddCommand(recordCmd)
}

func runRecord(args Arguments) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeconfig.ClientConfig()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	config.QPS = 1000
	config.Burst = 1000

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	discoveryClient := clientset.Discovery()

	// Get the list of all API resources available
	serverResources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		if discovery.IsGroupDiscoveryFailedError(err) {
			fmt.Printf("WARNING: The Kubernetes server has an orphaned API service. Server reports: %s\n", err.Error())
			fmt.Printf("WARNING: To fix this, kubectl delete apiservice <service-name>\n")
		} else {
			panic(err)
		}
	}

	createRecorders(context.TODO(), serverResources, args, dynClient)
}

func createRecorders(ctx context.Context, serverResources []*metav1.APIResourceList, args Arguments, dynClient *dynamic.DynamicClient) {
	var wg sync.WaitGroup
	for _, resourceList := range serverResources {
		groupVersion, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse group version: %v\n", err)
			continue
		}
		for i := range resourceList.APIResources {
			resourceName := resourceList.APIResources[i].Name
			if slices.Contains(resourcesToSkip, groupResource{groupVersion.Group, resourceName}) {
				continue
			}
			go watchGVR(ctx, &args, dynClient, schema.GroupVersionResource{
				Group:    groupVersion.Group,
				Version:  groupVersion.Version,
				Resource: resourceName,
			})
		}
	}
	wg.Add(1)
	wg.Wait()
}

type groupResource struct {
	group    string
	resource string
}

var resourcesToSkip = []groupResource{
	{"authentication.k8s.io", "tokenreviews"},
	{"authorization.k8s.io", "localsubjectaccessreviews"},
	{"authorization.k8s.io", "subjectaccessreviews"},
	{"authorization.k8s.io", "selfsubjectrulesreviews"},
	{"authorization.k8s.io", "selfsubjectaccessreviews"},
	{"", "componentstatuses"},
	{"", "bindings"},
	{"metallb.io", "addresspools"},
}

func watchGVR(ctx context.Context, args *Arguments, dynClient *dynamic.DynamicClient, gvr schema.GroupVersionResource) error {
	watch, err := dynClient.Resource(gvr).Watch(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("..Error watching %v. group %q version %q resource %q\n", err,
			gvr.Group, gvr.Version, gvr.Resource)
		return err
	}
	defer watch.Stop()
	for {
		select {
		case event := <-watch.ResultChan():
			handleEvent(event)
		case <-ctx.Done():
			return nil
		}
	}
}

func handleEvent(event watch.Event) {
	gvk := event.Object.GetObjectKind().GroupVersionKind()
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		fmt.Printf("Internal Error, could not cast to Unstructered %T %+v\n", event.Object, event.Object)
		return
	}
	switch event.Type {
	case watch.Added:
		fmt.Printf("%s %s %s\n", event.Type, gvk.Kind, gvk.Group)
		storeResource(gvk.Group, gvk.Version, gvk.Kind, obj)
	case watch.Modified:
		//json, _ := obj.MarshalJSON()
		fmt.Printf("%s %s %s %q\n", event.Type, gvk.Kind, gvk.Group, getString(obj, "metadata", "name"))
	case watch.Deleted:
		fmt.Printf("%s %s\n", event.Type, event.Object)
	case watch.Bookmark:
		fmt.Printf("%s %s\n", event.Type, event.Object)
	case watch.Error:
		fmt.Printf("%s %s\n", event.Type, event.Object)
	default:
		fmt.Printf("Internal Error, unknown event %s %s\n", event.Type, event.Object)
	}
}

func storeResource(group string, version string, kind string, obj *unstructured.Unstructured) {
	bytes, err := yaml.Marshal(obj)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(bytes))
}

func getString(obj *unstructured.Unstructured, fields ...string) string {
	val, found, err := unstructured.NestedString(obj.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return val
}
