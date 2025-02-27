package record

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

const TimeFormat = "20060102-150405.00000"

type Arguments struct {
	Verbose         bool
	OutputDirectory string
}

func RunRecordWithContext(ctx context.Context, args Arguments, kubeconfig clientcmd.ClientConfig) (*sync.WaitGroup, error) {
	config, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig.ClientConfig() failed: %w", err)
	}

	config.QPS = -1
	config.Burst = -1

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes.NewForConfig() failed: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("dynamic.NewForConfig() failed: %w", err)
	}

	discoveryClient := clientset.Discovery()

	// Get the list of all API resources available
	serverResources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		if discovery.IsGroupDiscoveryFailedError(err) {
			fmt.Printf("WARNING: The Kubernetes server has an orphaned API service. Server reports: %s\n", err.Error())
			fmt.Printf("WARNING: To fix this, kubectl delete apiservice <service-name>\n")
		} else {
			return nil, fmt.Errorf("discoveryClient.ServerPreferredResources() failed: %w", err)
		}
	}
	host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(config.Host, "https://"), "http://"), ":443")

	return createRecorders(ctx, serverResources, args, dynClient, host)
}

func createRecorders(ctx context.Context, serverResources []*metav1.APIResourceList, args Arguments, dynClient *dynamic.DynamicClient, host string) (*sync.WaitGroup, error) {
	var wg sync.WaitGroup

	recordFile := filepath.Join(args.OutputDirectory, "record-"+time.Now().Format(TimeFormat))

	err := os.WriteFile(recordFile, []byte(""), 0o600)
	if err != nil {
		return nil, fmt.Errorf("os.WriteFile() failed %q: %w", recordFile, err)
	}

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
			wg.Add(1)
			go watchGVR(ctx, &wg, &args, dynClient, schema.GroupVersionResource{
				Group:    groupVersion.Group,
				Version:  groupVersion.Version,
				Resource: resourceName,
			}, host)
		}
	}
	return &wg, nil
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
	{"", "events"}, // exists twice. Second time with group events.k8s.io
	{"metallb.io", "addresspools"},
	{"coordination.k8s.io", "leases"},                     // Leases create too many modifications
	{"apiextensions.k8s.io", "customresourcedefinitions"}, //
}

// watchGVR is called as Goroutine. It prints errors.
func watchGVR(ctx context.Context, wg *sync.WaitGroup, args *Arguments, dynClient *dynamic.DynamicClient, gvr schema.GroupVersionResource, host string) {
	defer wg.Done()
	fmt.Printf("Watching %q %q\n", gvr.Group, gvr.Resource)

	watch, err := dynClient.Resource(gvr).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("..Error watching %v. group %q version %q resource %q\n", err,
			gvr.Group, gvr.Version, gvr.Resource)
		return
	}
	defer watch.Stop()
	for {
		select {
		case event, ok := <-watch.ResultChan():
			if !ok {
				// If there are not objects in a resource, the watch gets closed.
				return
			}
			err := handleEvent(args, gvr, event, host)
			if err != nil {
				fmt.Printf("Error handling event: %v\n", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func handleEvent(args *Arguments, gvr schema.GroupVersionResource, event watch.Event, host string) error {
	if event.Object == nil {
		return fmt.Errorf("event.Object is nil? Skipping this event. Type=%s %+v gvr: (group=%s version=%s resource=%s)", event.Type, event,
			gvr.Group, gvr.Version, gvr.Resource)
	}
	gvk := event.Object.GetObjectKind().GroupVersionKind()
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("internal Error, could not cast to Unstructered %T %+v", event.Object, event.Object)
	}
	switch event.Type {
	case watch.Modified, watch.Added, watch.Deleted, watch.Bookmark, watch.Error:
		fmt.Printf("%s %s %s/%s\n", event.Type, gvk.Kind,
			getString(obj, "metadata", "namespace"),
			getString(obj, "metadata", "name"),
		)
		if err := storeResource(args, gvk.Group, gvk.Kind, obj, host); err != nil {
			return fmt.Errorf("error storing resource: %w", err)
		}
	default:
		fmt.Printf("Internal Error, unknown event %s %+v %s\n", event.Type, gvk, event.Object)
	}
	return nil
}

func storeResource(args *Arguments, group string, kind string, obj *unstructured.Unstructured, host string) error {
	bytes, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("yaml.Marshal(obj) failed: %w", err)
	}
	name := getString(obj, "metadata", "name")
	if name == "" {
		return fmt.Errorf("obj has no name? %+v", obj)
	}
	ns := getString(obj, "metadata", "namespace")
	dir := filepath.Join(args.OutputDirectory, host, group, kind, ns, name)
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return fmt.Errorf("os.MkdirAll() failed: %w", err)
	}
	file := filepath.Join(dir, time.Now().Format(TimeFormat)+".yaml")
	if err := os.WriteFile(file, bytes, 0o600); err != nil {
		return fmt.Errorf("os.WriteFile() failed: %w", err)
	}
	return nil
}

func getString(obj *unstructured.Unstructured, fields ...string) string {
	val, found, err := unstructured.NestedString(obj.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return val
}
