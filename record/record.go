package record

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/guettli/watchall/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	_ "modernc.org/sqlite"
)

func RunRecordWithContext(ctx context.Context, wg *sync.WaitGroup, args config.Arguments, config *restclient.Config, host string) error {
	// This might increase performance, but we do that many api-calls at the moment.
	// config.QPS = 1000
	// config.Burst = 1000

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}

	discoveryClient := clientset.Discovery()

	// Get the list of all API resources available
	serverResources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		if discovery.IsGroupDiscoveryFailedError(err) {
			fmt.Printf("WARNING: The Kubernetes server has an orphaned API service. Server reports: %s\n", err.Error())
			fmt.Printf("WARNING: To fix this, kubectl delete apiservice <service-name>\n")
		} else {
			return err
		}
	}
	return createRecorders(ctx, wg, serverResources, args, dynClient, host)
}

func createRecorders(ctx context.Context, wg *sync.WaitGroup, serverResources []*metav1.APIResourceList, args config.Arguments, dynClient *dynamic.DynamicClient, host string) error {
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
			go watchGVR(ctx, wg, &args, dynClient, schema.GroupVersionResource{
				Group:    groupVersion.Group,
				Version:  groupVersion.Version,
				Resource: resourceName,
			}, host)
		}
	}
	return nil
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

func watchGVR(ctx context.Context, wg *sync.WaitGroup, args *config.Arguments, dynClient *dynamic.DynamicClient, gvr schema.GroupVersionResource, host string) (reterr error) {
	defer func() {
		// Could not find a good way to stop all Go routine.
		// Still looking for a better solution.
		// https://stackoverflow.com/questions/61518410
		if reterr != nil {
			fmt.Printf("Watching GroupVersionResource failed (%s): %s", gvr, reterr.Error())
		}
	}()
	defer wg.Done()
	fmt.Printf("Watching %q %q\n", gvr.Group, gvr.Resource)

	// TODO: Use SendInitialEvents to avoid getting the old state.
	watch, err := dynClient.Resource(gvr).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("..Error watching %v. group %q version %q resource %q\n", err,
			gvr.Group, gvr.Version, gvr.Resource)
		return err
	}
	defer watch.Stop()
	for {
		select {
		case event, ok := <-watch.ResultChan():
			if !ok {
				// If there are not objects in a resource, the watch gets closed.
				return nil
			}
			err = handleEvent(args, gvr, event, host)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func handleEvent(args *config.Arguments, gvr schema.GroupVersionResource, event watch.Event, host string) error {
	if event.Object == nil {
		fmt.Printf("event.Object is nil? Waiting a moment and skipping this event. Type=%s %+v gvr: (group=%s version=%s resource=%s)\n", event.Type, event,
			gvr.Group, gvr.Version, gvr.Resource)
		time.Sleep(10 * time.Second)
		return nil
	}
	gvk := event.Object.GetObjectKind().GroupVersionKind()
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		fmt.Printf("Internal Error, could not cast to Unstructered %T %+v\n", event.Object, event.Object)
		return nil
	}
	switch event.Type {
	case watch.Added:
		fmt.Printf("%s %s %s\n", event.Type, gvk.Kind, gvk.Group)
		storeResource(args, gvk.Group, gvk.Version, gvk.Kind, obj, host)

	case watch.Modified:
		// json, _ := obj.MarshalJSON()
		fmt.Printf("%s %s %s %q\n", event.Type, gvk.Kind, gvk.Group, getString(obj, "metadata", "name"))
		storeResource(args, gvk.Group, gvk.Version, gvk.Kind, obj, host)
	case watch.Deleted:
		fmt.Printf("%s %+v %s\n", event.Type, gvk, event.Object)
	case watch.Bookmark:
		fmt.Printf("%s %+v %s\n", event.Type, gvk, event.Object)
	case watch.Error:
		fmt.Printf("%s %+v %s\n", event.Type, gvk, event.Object)
	default:
		fmt.Printf("Internal Error, unknown event %s %+v %s\n", event.Type, gvk, event.Object)
	}
	return nil
}

func storeResource(args *config.Arguments, group string, version string, kind string, obj *unstructured.Unstructured, host string) {
	args.StoreChannel <- obj
}

func HandleStoreChannel(ctx context.Context, args *config.Arguments) {
	for {
		select {
		case <-ctx.Done():
			return
		case obj := <-args.StoreChannel:
			if ctx.Err() != nil {
				return
			}
			jsonResource, err := json.Marshal(obj)
			if err != nil {
				args.FatalErrorChannel <- err
				return
			}
			_, err = args.Db.Exec(`
	INSERT INTO res (
		apiVersion,
		name,
		namespace,
		creationTimestamp,
		kind,
		resourceVersion,
		uid,
		json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				getString(obj, "apiVersion"),
				getString(obj, "metadata", "name"),
				getString(obj, "metadata", "namespace"),
				getString(obj, "metadata", "creationTimestamp"),
				getString(obj, "kind"),
				getString(obj, "metadata", "resourceVersion"),
				getString(obj, "metadata", "uid"),
				string(jsonResource))
			if err != nil {
				args.FatalErrorChannel <- err
				return
			}
		}
	}
}

func getString(obj *unstructured.Unstructured, fields ...string) string {
	val, found, err := unstructured.NestedString(obj.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return val
}
