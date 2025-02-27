package deltas

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/akedrou/textdiff"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var resourcesToSkip = []string{
	"events.k8s.io/Event",
}

type fileType struct {
	basename string
	path     string
}

func (f fileType) String() string {
	return filepath.Join(f.path, f.basename)
}

func Deltas(dir string, skipPatterns []string) error {
	dir = filepath.Clean(dir)
	skipRegex := make([]*regexp.Regexp, 0, len(skipPatterns))
	for _, pattern := range skipPatterns {
		r, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("regexp.Compile() failed: %q %w", pattern, err)
		}
		skipRegex = append(skipRegex, r)
	}
	records, err := filepath.Glob(filepath.Join(dir, "record-*"))
	if err != nil {
		return fmt.Errorf("os.Glob() failed: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("No record-YYYYMM... file found in %s", dir)
	}
	slices.Sort(records)
	record := records[len(records)-1]
	startTimestamp := strings.SplitN(filepath.Base(record), "-", 2)[1]

	var files []fileType

	err = filepath.WalkDir(dir, func(path string, info os.DirEntry, err error) error {
		if info.IsDir() {
			return nil
		}
		if filepath.Dir(path) == dir {
			return nil
		}
		if info.Name() < startTimestamp {
			return nil
		}
		for _, r := range skipRegex {
			if r.MatchString(path) {
				return nil
			}
		}
		p, err := filepath.Rel(dir, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("filepath.Rel() failed: %w", err)
		}
		files = append(files, fileType{
			basename: info.Name(),
			path:     p,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("filepath.WalkDir() failed: %w", err)
	}
	fmt.Printf("Found %d files for timestamp %s\n", len(files), startTimestamp)

	sort.Slice(files, func(i, j int) bool {
		return files[i].basename < files[j].basename
	})
	for _, file := range files {
		err := showFile(dir, file, startTimestamp)
		if err != nil {
			return fmt.Errorf("showFile() failed: %w", err)
		}
	}
	return nil
}

func showFile(dir string, file fileType, startTimestamp string) error {
	for _, resource := range resourcesToSkip {
		if strings.HasPrefix(file.path, resource+string(filepath.Separator)) {
			fmt.Printf("Skipping %s\n", file.String())
			return nil
		}
	}

	// fmt.Printf("File: %s\n", file.String())

	absDir := filepath.Join(dir, file.path)
	// find previous file
	dirEntries, err := os.ReadDir(absDir)
	if err != nil {
		return fmt.Errorf("os.ReadDir() failed: %w", err)
	}
	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() > dirEntries[j].Name()
	})

	found := false
	previous := ""
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		if found {
			previous = entry.Name()
		}
		if entry.Name() == file.basename {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("internal error. Not found: %q %s", file.path, file.basename)
	}
	if previous == "" {
		fmt.Printf("No previous file found: %s\n", file.String())
		return nil
	}
	if previous < startTimestamp {
		// fmt.Printf("Skipping because before %s %s\n", startTimestamp, previous)
		return nil
	}
	compareTwoYamlFiles(filepath.Join(absDir, previous), filepath.Join(absDir, file.basename))
	return nil
}

func compareTwoYamlFiles(f1, f2 string) error {
	yaml1, err := os.ReadFile(f1)
	if err != nil {
		panic(fmt.Errorf("failed to read %q: %w", f1, err))
	}

	yaml2, err := os.ReadFile(f2)
	if err != nil {
		panic(fmt.Errorf("failed to read %q: %w", f2, err))
	}

	// Decode the YAML into unstructured objects
	obj1, err := yamlToUnstructured(yaml1)
	if err != nil {
		return fmt.Errorf("failed to decode first YAML: %w", err)
	}

	obj2, err := yamlToUnstructured(yaml2)
	if err != nil {
		return fmt.Errorf("failed to decode second YAML: %w", err)
	}

	// Strip irrelevant fields (like resourceVersion)
	if err := stripIrrelevantFields(obj1); err != nil {
		return fmt.Errorf("stripIrrelevantFields failed %q: %w", f1, err)
	}
	if err := stripIrrelevantFields(obj2); err != nil {
		return fmt.Errorf("stripIrrelevantFields failed %q: %w", f2, err)
	}

	// Compare the objects
	if equality.Semantic.DeepEqual(obj1, obj2) {
		return nil
	}
	s1, err := unstructuredToString(obj1)
	if err != nil {
		return fmt.Errorf("unstructuredToString failed %q: %w", f1, err)
	}
	s2, err := unstructuredToString(obj2)
	if err != nil {
		return fmt.Errorf("unstructuredToString failed %q: %w", f2, err)
	}

	diff := textdiff.Unified(f1, f2, s1, s2)
	fmt.Printf("Diff of %q %q\n%s\n\n", f1, f2, diff)
	return nil
}

func unstructuredToString(obj *unstructured.Unstructured) (string, error) {
	serializer := json.NewYAMLSerializer(json.DefaultMetaFactory, nil, nil)
	var buffer bytes.Buffer
	err := serializer.Encode(obj, &buffer)
	if err != nil {
		return "", fmt.Errorf("failed to serialize to YAML: %w", err)
	}
	return buffer.String(), nil
}

func yamlToUnstructured(yamlData []byte) (*unstructured.Unstructured, error) {
	// Convert YAML to JSON
	jsonData, err := yaml.ToJSON(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}

	// Unmarshal JSON into an unstructured.Unstructured object
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(jsonData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to Unstructured: %w", err)
	}

	return obj, nil
}

func stripIrrelevantFields(obj *unstructured.Unstructured) error {
	// Remove metadata fields that are not relevant
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj.Object, "metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration")
	return nil
}
