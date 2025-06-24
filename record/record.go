package record

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

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

type IgnoreLogLine struct {
	FileRegex *regexp.Regexp
	LineRegex *regexp.Regexp
}
type Arguments struct {
	Verbose                  bool
	OutputDirectory          string
	WithLogs                 bool
	DisableResourceRecording bool
	IgnoreLogLinesFile       string
	IgnoreLogLines           []IgnoreLogLine
	IgnorePods               []*regexp.Regexp
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
	var wg sync.WaitGroup

	if !args.DisableResourceRecording {
		err = createRecorders(ctx, &wg, serverResources, args, dynClient, host)
		if err != nil {
			return nil, fmt.Errorf("createRecorders() failed: %w", err)
		}
	}

	if args.WithLogs {
		err = createLogScraper(ctx, &wg, clientset, args, host)
		if err != nil {
			return nil, fmt.Errorf("createLogScraper() failed: %w", err)
		}
	}
	return &wg, nil
}

func createLogScraper(ctx context.Context, wg *sync.WaitGroup,
	clientset *kubernetes.Clientset, args Arguments, host string,
) error {
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("clientset.CoreV1().Pods().List() failed: %w", err)
	}

	for _, pod := range pods.Items {
		skip := false
		for _, ignorePod := range args.IgnorePods {
			if ignorePod.MatchString(pod.Name) {
				fmt.Printf("Skipping pod %s/%s because it matches ignore-pod-regex %q\n", pod.Namespace, pod.Name, ignorePod.String())
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		var regexOfThisPod []*regexp.Regexp
		for _, ignoreLine := range args.IgnoreLogLines {
			if ignoreLine.FileRegex.MatchString(pod.Name) {
				regexOfThisPod = append(regexOfThisPod, ignoreLine.LineRegex)
			}
		}
		for _, container := range pod.Spec.Containers {
			wg.Add(1)
			go readPodLogs(ctx, wg, clientset, args, host, pod.Name, pod.Namespace,
				container.Name, regexOfThisPod)
		}
	}
	return nil
}

func readPodLogs(ctx context.Context, wg *sync.WaitGroup, clientset *kubernetes.Clientset, args Arguments, host, podName, namespace, containerName string, ignoreLineRegexs []*regexp.Regexp) {
	defer wg.Done()
	fmt.Printf("Watching logs for pod %s/%s container %s\n", namespace, podName, containerName)
	stream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName,
		&corev1.PodLogOptions{
			Container:    containerName,
			Follow:       true,
			SinceSeconds: ptr.To(int64(1)),
		},
	).Stream(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error streaming logs for %s/%s [%s]: %v\n", namespace, podName, containerName, err)
		return
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		line := scanner.Text()
		for _, ignoreLineRegex := range ignoreLineRegexs {
			if ignoreLineRegex.MatchString(line) {
				fmt.Printf("Ignoring log line for pod %s/%s container %s: %q\n", namespace, podName, containerName, line)
				continue
			}
		}
		dir := filepath.Join(args.OutputDirectory, host, "core", "Pod", namespace, podName)
		err = os.MkdirAll(dir, 0o700)
		if err != nil {
			fmt.Printf("Error creating directory %s: %s\n", dir, err)
			continue
		}

		file := filepath.Join(dir, time.Now().UTC().Format(TimeFormat)+".log")

		if err := os.WriteFile(file, []byte(line+"\n"), 0o600); err != nil {
			fmt.Printf("Error writing log file %s: %s\n", file, err)
			continue
		}
		fmt.Printf("Created log file %s for pod %s/%s container %s\n", file, namespace, podName, containerName)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading logs for %s/%s [%s]: %v\n", namespace, podName, containerName, err)
	}
}

func createRecorders(ctx context.Context, wg *sync.WaitGroup, serverResources []*metav1.APIResourceList, args Arguments, dynClient *dynamic.DynamicClient, host string) error {
	baseDir := filepath.Join(args.OutputDirectory, host)
	err := os.MkdirAll(baseDir, 0o700)
	if err != nil {
		return fmt.Errorf("os.MkdirAll() failed: %w", err)
	}

	// the recordFile creates a marker file, so that the current time gets recorded.
	// This is useful to find out when the recording started.
	recordFile := filepath.Join(baseDir, "record-"+time.Now().UTC().Format(TimeFormat))

	err = os.WriteFile(recordFile, []byte(""), 0o600)
	if err != nil {
		return fmt.Errorf("os.WriteFile() failed %q: %w", recordFile, err)
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
	{"coordination.k8s.io", "leases"}, // Leases create too many modifications
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

func redactSecret(obj *unstructured.Unstructured) {
	for _, key := range []string{"data", "stringData"} {
		m, found, err := unstructured.NestedStringMap(obj.Object, key)
		if !found || err != nil {
			continue
		}
		for k, v := range m {
			if v == "" {
				continue
			}
			m[k] = fmt.Sprintf("redacted-to-sha256:%x", sha256.Sum256([]byte(v)))
		}
		err = unstructured.SetNestedStringMap(obj.Object, m, key)
		if err != nil {
			continue
		}
	}
}

func storeResource(args *Arguments, group string, kind string, obj *unstructured.Unstructured, host string) error {
	if group == "" && kind == "Secret" {
		redactSecret(obj)
	}
	bytes, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("yaml.Marshal(obj) failed: %w", err)
	}
	name := getString(obj, "metadata", "name")
	if name == "" {
		return fmt.Errorf("obj has no name? %+v", obj)
	}
	ns := getString(obj, "metadata", "namespace")
	if group == "" {
		group = "core"
	}
	dir := filepath.Join(args.OutputDirectory, host, group, kind, ns, name)
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return fmt.Errorf("os.MkdirAll() failed: %w", err)
	}
	file := filepath.Join(dir, time.Now().UTC().Format(TimeFormat)+".yaml")
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
