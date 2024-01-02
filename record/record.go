package record

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
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
	"k8s.io/client-go/tools/clientcmd"
	_ "modernc.org/sqlite"
)

func RunRecordWithContext(ctx context.Context, wg *sync.WaitGroup, args config.Arguments, kubeconfig clientcmd.ClientConfig) error {
	config, err := kubeconfig.ClientConfig()
	if err != nil {
		return err
	}

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
	host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(config.Host, "https://"), "http://"), ":443")
	fn := host + ".sqlite"
	db, err := sql.Open("sqlite", fn)
	if err != nil {
		return err
	}
	err = migrateDatabase(db)
	if err != nil {
		return err
	}

	return createRecorders(context.TODO(), wg, db, serverResources, args, dynClient, host)
}

func migrateDatabase(db *sql.DB) error {
	v := 0
	err := db.QueryRow("pragma user_version").Scan(&v)
	if err != nil {
		return err
	}
	for ; v < 1; v++ {
		var err error
		switch v {
		case 0:
			err = migrationToSchema0(db)
		default:
			panic(fmt.Sprintf("I am confused. No matching schema migration found. %d", v))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func migrationToSchema0(db *sql.DB) error {
	_, err := db.Exec(`
	BEGIN;
	CREATE TABLE res (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		apiVersion TEXT,
		name TEXT,
		namespace TEXT,
		creationTimestamp TEXT,
		kind TEXT,
		resourceVersion TEXT,
		uid TEXT,
		json TEXT);
		CREATE INDEX idx_apiversion ON res(apiVersion);
		CREATE INDEX idx_name ON res(name);
		CREATE INDEX idx_namespace ON res(namespace);
		CREATE INDEX idx_creationTimestamp ON res(creationTimestamp);
		CREATE INDEX idx_kind ON res(kind);
		CREATE INDEX idx_resourceVersion ON res(resourceVersion);
		CREATE INDEX idx_uid ON res(uid);
		PRAGMA user_version = 1;
		COMMIT;
		`)
	return err
}

func createRecorders(ctx context.Context, wg *sync.WaitGroup, db *sql.DB, serverResources []*metav1.APIResourceList, args config.Arguments, dynClient *dynamic.DynamicClient, host string) error {
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
			go watchGVR(ctx, db, wg, &args, dynClient, schema.GroupVersionResource{
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

func watchGVR(ctx context.Context, db *sql.DB, wg *sync.WaitGroup, args *config.Arguments, dynClient *dynamic.DynamicClient, gvr schema.GroupVersionResource, host string) error {
	defer wg.Done()
	fmt.Printf("Watching %q %q\n", gvr.Group, gvr.Resource)

	// TODO: Use SendInitialEvents to avoid getting the old state.
	watch, err := dynClient.Resource(gvr).Watch(context.TODO(), metav1.ListOptions{})
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
			handleEvent(db, args, gvr, event, host)
		case <-ctx.Done():
			return nil
		}
	}
}

func handleEvent(db *sql.DB, args *config.Arguments, gvr schema.GroupVersionResource, event watch.Event, host string) {
	if event.Object == nil {
		fmt.Printf("event.Object is nil? Waiting a moment and skipping this event. Type=%s %+v gvr: (group=%s version=%s resource=%s)\n", event.Type, event,
			gvr.Group, gvr.Version, gvr.Resource)
		time.Sleep(10 * time.Second)
		return
	}
	gvk := event.Object.GetObjectKind().GroupVersionKind()
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		fmt.Printf("Internal Error, could not cast to Unstructered %T %+v\n", event.Object, event.Object)
		return
	}
	switch event.Type {
	case watch.Added:
		fmt.Printf("%s %s %s\n", event.Type, gvk.Kind, gvk.Group)
		storeResource(db, args, gvk.Group, gvk.Version, gvk.Kind, obj, host)
	case watch.Modified:
		// json, _ := obj.MarshalJSON()
		fmt.Printf("%s %s %s %q\n", event.Type, gvk.Kind, gvk.Group, getString(obj, "metadata", "name"))
		storeResource(db, args, gvk.Group, gvk.Version, gvk.Kind, obj, host)
	case watch.Deleted:
		fmt.Printf("%s %+v %s\n", event.Type, gvk, event.Object)
	case watch.Bookmark:
		fmt.Printf("%s %+v %s\n", event.Type, gvk, event.Object)
	case watch.Error:
		fmt.Printf("%s %+v %s\n", event.Type, gvk, event.Object)
	default:
		fmt.Printf("Internal Error, unknown event %s %+v %s\n", event.Type, gvk, event.Object)
	}
}

func storeResource(db *sql.DB, args *config.Arguments, group string, version string, kind string, obj *unstructured.Unstructured, host string) error {
	jsonResource, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
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
		jsonResource)
	return err
}

func getString(obj *unstructured.Unstructured, fields ...string) string {
	val, found, err := unstructured.NestedString(obj.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return val
}
