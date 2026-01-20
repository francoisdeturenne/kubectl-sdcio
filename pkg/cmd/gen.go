/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

type GenOptions struct {
	yangPath     string
	modelPath    string
	outputFormat string
	outputFile   string
	rootEntry    *yang.Entry
	namespace    string
	MyOptions
}

// for SDCIO Config
type SDCConfig struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   SDCMetadata `yaml:"metadata"`
	Spec       SDCSpec     `yaml:"spec"`
}

type SDCMetadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

type SDCSpec struct {
	Lifecycle SDCLifecycle    `yaml:"lifecycle"`
	Revertive bool            `yaml:"revertive"`
	Priority  int             `yaml:"priority"`
	Config    []SDCConfigItem `yaml:"config"`
}

type SDCLifecycle struct {
	DeletionPolicy string `yaml:"deletionPolicy"`
}

type SDCConfigItem struct {
	Path  string      `yaml:"path"`
	Value interface{} `yaml:"value"`
}

// KeyValue represents a key-value pair extracted from the path
type KeyValue struct {
	Name  string
	Value string
}

func stripKeysFromPath(path string) string {
	// Use regex to remove key expressions like [name=<key>], [tac=<key>], etc.
	re := regexp.MustCompile(`\[[^\]]+=<[^>]+>\]`)
	return re.ReplaceAllString(path, "")
}

// NewGenOptions provides an instance of GenOptions with default values
func NewGenOptions(streams genericiooptions.IOStreams) *GenOptions {
	return &GenOptions{
		modelPath:    "/",
		outputFormat: "json",
		MyOptions: MyOptions{
			IOStreams: streams,
		},
	}
}

func (o *GenOptions) Complete(_ *cobra.Command, _ []string) error {
	// Check that the YANG file exists
	if o.yangPath != "" {
		if _, err := os.Stat(o.yangPath); os.IsNotExist(err) {
			return fmt.Errorf("YANG file '%s' does not exist", o.yangPath)
		}
	}
	return nil
}

// Validate validates the options
func (o *GenOptions) Validate() error {
	if o.yangPath == "" {
		return fmt.Errorf("--yang parameter is required")
	}

	// Validate output format
	validFormats := []string{"json", "yaml", "xml", "sdc-conf"}
	formatValid := false
	for _, format := range validFormats {
		if strings.ToLower(o.outputFormat) == format {
			formatValid = true
			break
		}
	}
	if !formatValid {
		return fmt.Errorf("invalid output format '%s'. Valid formats: %s",
			o.outputFormat, strings.Join(validFormats, ", "))
	}

	return nil
}

func (o *GenOptions) Run(_ *cobra.Command) error {
	// Generate template from YANG model
	template, err := o.generateTemplateFromYang()
	if err != nil {
		return fmt.Errorf("failed to generate template: %v", err)
	}

	// Output the result
	return o.outputTemplate(template)
}

func (o *GenOptions) generateTemplateFromYang() (map[string]interface{}, error) {
	// Create a new module set
	ms := yang.NewModules()

	// Load the YANG file
	if err := ms.Read(o.yangPath); err != nil {
		return nil, fmt.Errorf("failed to read YANG file: %v", err)
	}

	// Process the modules
	if errs := ms.Process(); len(errs) > 0 {
		var errMsgs []string
		for _, err := range errs {
			errMsgs = append(errMsgs, err.Error())
		}
		return nil, fmt.Errorf("failed to process YANG modules: %s", strings.Join(errMsgs, "; "))
	}

	// Find the main module
	var mainModule *yang.Module

	for _, module := range ms.Modules {
		if module != nil {
			// Check if this module was loaded from our target file
			if strings.HasPrefix(filepath.Base(o.yangPath), module.Name) {
				mainModule = module
				break
			}

		}
	}

	if mainModule == nil {
		return nil, fmt.Errorf("no valid module found in YANG file")
	}

	// Create root entry from module
	rootEntry := yang.ToEntry(mainModule)
	if rootEntry == nil {
		return nil, fmt.Errorf("failed to convert module to entry")
	}

	// Find the entry corresponding to the specified path
	cleanPath := stripKeysFromPath(o.modelPath)
	entry := o.findEntryByPath(rootEntry, cleanPath)
	if entry == nil {
		return nil, fmt.Errorf("path '%s' not found in YANG model", o.modelPath)
	}
	// Store root entry and namespace for XML generation
	o.rootEntry = rootEntry
	if mainModule.Namespace != nil {
		o.namespace = mainModule.Namespace.Name
	}

	// Generate the template
	template := o.generateTemplate(entry)
	// Extract and remove keys from path for format sdc and xml
	keysToExclude := o.extractKeysFromPath(o.modelPath)
	if len(keysToExclude) > 0 && (o.outputFormat == "sdc-conf" || o.outputFormat == "xml") {
		template = o.removeKeysFromTemplate(template, keysToExclude)
	}
	return template, nil
}

func (o *GenOptions) findEntryByPath(rootEntry *yang.Entry, path string) *yang.Entry {
	// Clean the path
	path = strings.Trim(path, "/")

	// If path is empty or root, return the root entry
	if path == "" || path == "/" {
		return rootEntry
	}

	// Split path into parts
	parts := strings.Split(path, "/")
	current := rootEntry

	// Navigate through the tree
	for _, part := range parts {
		if current.Dir == nil {
			return nil
		}

		found := false
		for _, child := range current.Dir {
			if child.Name == part {
				current = child
				found = true
				break
			}
		}

		if !found {
			return nil
		}
	}

	return current
}

func (o *GenOptions) generateTemplate(entry *yang.Entry) map[string]interface{} {
	if entry == nil {
		return nil
	}

	result := make(map[string]interface{})

	switch entry.Kind {
	case yang.DirectoryEntry:
		// Container or module
		if entry.Dir != nil {
			for _, child := range entry.Dir {
				childTemplate := o.processEntry(child)
				if childTemplate != nil {
					result[child.Name] = childTemplate
				}
			}
		}

	default:
		// For non-directory entries, process the entry and wrap in a map if needed
		processed := o.processEntry(entry)
		if processed != nil {
			// If the processed entry is already a map, return it directly
			if mapResult, ok := processed.(map[string]interface{}); ok {
				return mapResult
			}
			// Otherwise, wrap it in a map with the entry name as key
			result[entry.Name] = processed
		}
	}

	return result
}

func (o *GenOptions) processEntry(entry *yang.Entry) interface{} {
	if entry == nil {
		return nil
	}

	switch entry.Kind {
	case yang.DirectoryEntry:
		// Container
		container := make(map[string]interface{})
		if entry.Dir != nil {
			for _, child := range entry.Dir {
				childTemplate := o.processEntry(child)
				if childTemplate != nil {
					container[child.Name] = childTemplate
				}
			}
		}
		return container

	case yang.LeafEntry:
		// Simple leaf
		return o.generateLeafTemplate(entry)

	case yang.ChoiceEntry:
		// Choice entry - generate template for first case
		if len(entry.Dir) > 0 {
			// Get the first case from the map
			for _, firstCase := range entry.Dir {
				return o.processEntry(firstCase)
			}
		}
		return map[string]interface{}{"_choice": "select_case"}

	case yang.CaseEntry:
		// Case entry - process its children
		container := make(map[string]interface{})
		if entry.Dir != nil {
			for _, child := range entry.Dir {
				childTemplate := o.processEntry(child)
				if childTemplate != nil {
					container[child.Name] = childTemplate
				}
			}
		}
		return container

	case yang.AnyDataEntry:
		return map[string]interface{}{"_anydata": "any_data_value"}

	case yang.AnyXMLEntry:
		return map[string]interface{}{"_anyxml": "any_xml_value"}

	case yang.NotificationEntry:
		// Notification entry
		notification := make(map[string]interface{})
		if entry.Dir != nil {
			for _, child := range entry.Dir {
				childTemplate := o.processEntry(child)
				if childTemplate != nil {
					notification[child.Name] = childTemplate
				}
			}
		}
		return map[string]interface{}{"_notification": notification}

	case yang.InputEntry, yang.OutputEntry:
		// RPC input/output
		rpcData := make(map[string]interface{})
		if entry.Dir != nil {
			for _, child := range entry.Dir {
				childTemplate := o.processEntry(child)
				if childTemplate != nil {
					rpcData[child.Name] = childTemplate
				}
			}
		}
		return rpcData

	default:
		// Check if it's a list by examining the ListAttr field
		if entry.ListAttr != nil {
			return o.generateListTemplate(entry)
		}
		return fmt.Sprintf("<!-- %s: %s -->", entry.Kind, entry.Name)
	}
}

func (o *GenOptions) generateLeafTemplate(entry *yang.Entry) interface{} {
	if entry.Type == nil {
		return "value"
	}

	// Check if it's a leaf-list
	if entry.ListAttr != nil {
		leafValue := o.getDefaultValueForType(entry.Type)
		return []interface{}{leafValue}
	}

	// Add constraint information if available
	value := o.getDefaultValueForType(entry.Type)

	// If there are constraints, add them as comments
	if len(entry.Type.Range) > 0 {
		return fmt.Sprintf("%v <!-- range: %v -->", value, entry.Type.Range)
	}

	if len(entry.Type.Pattern) > 0 {
		return fmt.Sprintf("%v <!-- pattern: %s -->", value, entry.Type.Pattern[0])
	}

	return value
}

func (o *GenOptions) getDefaultValueForType(yangType *yang.YangType) interface{} {
	switch yangType.Kind {
	case yang.Ystring:
		return "string_value"
	case yang.Yint8, yang.Yint16, yang.Yint32, yang.Yint64:
		return 0
	case yang.Yuint8, yang.Yuint16, yang.Yuint32, yang.Yuint64:
		return 0
	case yang.Ybool:
		return false
	case yang.Ydecimal64:
		return 0.0
	case yang.Yenum:
		if yangType.Enum != nil && len(yangType.Enum.Names()) > 0 {
			return yangType.Enum.Names()[0]
		}
		return "enum_value"
	case yang.Yidentityref:
		return "identity_value"
	case yang.Ybinary:
		return "base64_encoded_value"
	case yang.Yempty:
		return ""
	case yang.Yunion:
		return "union_value"
	case yang.Yleafref:
		return "leafref_value"
	default:
		if yangType.Name != "" {
			return fmt.Sprintf("value_of_type_%s", yangType.Name)
		}
		return "unknown_value"
	}
}

// SDC Template
func (o *GenOptions) generateSDCConfig(template map[string]interface{}) SDCConfig {
	// Generate name with path suffix
	name := "gen-sdcio-config"

	if o.modelPath != "/" && o.modelPath != "" {
		// Clean the path and get the last part
		cleanPath := stripKeysFromPath(strings.Trim(o.modelPath, "/"))
		if cleanPath != "" {
			pathParts := strings.Split(cleanPath, "/")
			lastPart := pathParts[len(pathParts)-1]
			name = fmt.Sprintf("%s-%s", name, lastPart)
		}
	}

	return SDCConfig{
		APIVersion: "config.sdcio.dev/v1alpha1",
		Kind:       "Config",
		Metadata: SDCMetadata{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				"config.sdcio.dev/targetName":      "targetName",
				"config.sdcio.dev/targetNamespace": "default",
			},
		},
		Spec: SDCSpec{
			Lifecycle: SDCLifecycle{
				DeletionPolicy: "orphan",
			},
			Revertive: true,
			Priority:  100,
			Config: []SDCConfigItem{
				{
					Path:  o.modelPath,
					Value: template,
				},
			},
		},
	}
}

func (o *GenOptions) generateListTemplate(entry *yang.Entry) map[string]interface{} {
	listTemplate := make(map[string]interface{})

	// Add list metadata
	if entry.Key != "" {
		listTemplate["_keys"] = strings.Split(entry.Key, " ")
	}

	// Add list information
	listTemplate["_type"] = "list"
	if entry.ListAttr != nil {
		//if entry.ListAttr.MinElements != nil {
		if entry.ListAttr != nil {
			listTemplate["_min_elements"] = (*entry.ListAttr).MinElements
		}
		//if entry.ListAttr.MaxElements != nil {
		if entry.ListAttr != nil {
			listTemplate["_max_elements"] = (*entry.ListAttr).MaxElements
		}
	}

	// Generate an example item
	listItem := make(map[string]interface{})
	if entry.Dir != nil {
		for _, child := range entry.Dir {
			childTemplate := o.processEntry(child)
			if childTemplate != nil {
				listItem[child.Name] = childTemplate
			}
		}
	}

	listTemplate["_list_item_example"] = listItem
	return listTemplate
}

func (o *GenOptions) outputTemplate(template map[string]interface{}) error {
	var output []byte
	var err error

	switch strings.ToLower(o.outputFormat) {
	case "json":
		output, err = json.MarshalIndent(template, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %v", err)
		}

	case "yaml":
		output, err = yaml.Marshal(template)
		if err != nil {
			return fmt.Errorf("error marshaling YAML: %v", err)
		}

	case "xml":
		return o.outputXML(template, 0)

	case "sdc-conf":
		sdcConfig := o.generateSDCConfig(template)
		output, err = yaml.Marshal(sdcConfig)
		if err != nil {
			return fmt.Errorf("error marshaling SDC Config YAML: %v", err)
		}

	default:
		return fmt.Errorf("unsupported format: %s", o.outputFormat)
	}

	// Write to file or stdout
	if o.outputFile != "" {
		return os.WriteFile(o.outputFile, output, 0644)
	}

	fmt.Fprint(o.Out, string(output))
	return nil
}

func (o *GenOptions) outputXML(data interface{}, depth int) error {
	// If this is the root call (depth 0), generate the full path structure
	if depth == 0 {
		return o.outputXMLWithPath(data)
	}

	// For nested calls, use the simple output
	indent := strings.Repeat("  ", depth)

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if strings.HasPrefix(key, "_") {
				// Skip metadata for XML
				continue
			}
			fmt.Fprintf(o.Out, "%s<%s>\n", indent, key)
			o.outputXML(value, depth+1)
			fmt.Fprintf(o.Out, "%s</%s>\n", indent, key)
		}
	case []interface{}:
		for _, item := range v {
			o.outputXML(item, depth)
		}
	default:
		fmt.Fprintf(o.Out, "%s%v\n", indent, v)
	}
	return nil
}

func (o *GenOptions) outputXMLWithPath(data interface{}) error {
	// Parse the path to get all segments
	cleanPath := stripKeysFromPath(strings.Trim(o.modelPath, "/"))
	pathSegments := []string{}
	if cleanPath != "" && cleanPath != "/" {
		pathSegments = strings.Split(cleanPath, "/")
	}
	
	// Check if the last path segment matches a key in the data map
	if len(pathSegments) > 0 {
		if dataMap, ok := data.(map[string]interface{}); ok {
			lastSegment := pathSegments[len(pathSegments)-1]
			// If the data contains a key matching the last segment, remove it from path
			if _, exists := dataMap[lastSegment]; exists {
				pathSegments = pathSegments[:len(pathSegments)-1]
			}
		}
	}
	// Extract keys from the original path
	pathKeys := o.extractPathKeysWithValues(o.modelPath)

	// Start building XML with root element and namespace
	rootName := ""
	if o.rootEntry != nil {
		rootName = o.rootEntry.Name
	}
	if rootName == "" {
		rootName = "root"
	}

	// Write root element with namespace
	if o.namespace != "" {
		fmt.Fprintf(o.Out, "<%s xmlns=\"%s\">\n", rootName, o.namespace)
	} else {
		fmt.Fprintf(o.Out, "<%s>\n", rootName)
	}
	//remove last path segment if identical to last data element
	
	// Build the path structure
	o.buildXMLPath(pathSegments, pathKeys, data, 1)

	// Close root element
	fmt.Fprintf(o.Out, "</%s>\n", rootName)

	return nil
}

func (o *GenOptions) buildXMLPath(pathSegments []string, pathKeys map[string][]KeyValue, data interface{}, depth int) {
	indent := strings.Repeat(" ", depth)

	if len(pathSegments) == 0 {
		// We've reached the target path, output the actual data
		o.outputXMLData(data, depth)
		return
	}

	// Get current segment
	currentSegment := pathSegments[0]
	remainingSegments := pathSegments[1:]

	
	
	fmt.Fprintf(o.Out, "%s<%s>\n", indent, currentSegment)

	// Check if this segment has keys
	if keys, hasKeys := pathKeys[currentSegment]; hasKeys {
		// Output key elements
		for _, kv := range keys {
			fmt.Fprintf(o.Out, "%s <%s>%s</%s>\n", indent, kv.Name, kv.Value, kv.Name)
		}
	}

	// Continue with remaining path
	o.buildXMLPath(remainingSegments, pathKeys, data, depth+1)

	// Close current element
	fmt.Fprintf(o.Out, "%s</%s>\n", indent, currentSegment)
	
}

func (o *GenOptions) outputXMLData(data interface{}, depth int) {
	indent := strings.Repeat(" ", depth)

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if strings.HasPrefix(key, "_") {
				// Skip metadata for XML
				continue
			}
			fmt.Fprintf(o.Out, "%s<%s>\n", indent, key)
			o.outputXMLData(value, depth+1)
			fmt.Fprintf(o.Out, "%s</%s>\n", indent, key)
		}
	case []interface{}:
		for _, item := range v {
			o.outputXMLData(item, depth)
		}
	default:
		fmt.Fprintf(o.Out, "%s%v\n", indent, v)
	}
}

// extractPathKeysWithValues extracts keys and their placeholder values from the path
// Returns a map where the key is the list name and value is a slice of KeyValue pairs
func (o *GenOptions) extractPathKeysWithValues(path string) map[string][]KeyValue {
	result := make(map[string][]KeyValue)

	// Match patterns like registered-ue-per-ta-list[name=<key>]
	// or tac[serving-plmn=<key>,tac=<key>]
	re := regexp.MustCompile(`([^/\[]+)\[([^\]]+)\]`)
	matches := re.FindAllStringSubmatch(path, -1)

	for _, match := range matches {
		if len(match) > 2 {
			listName := match[1]
			keysStr := match[2]

			// Parse individual keys
			keyPairs := strings.Split(keysStr, ",")
			var keyValues []KeyValue

			for _, keyPair := range keyPairs {
				parts := strings.Split(keyPair, "=")
				if len(parts) == 2 {
					keyName := strings.TrimSpace(parts[0])
					keyValue := strings.Trim(strings.TrimSpace(parts[1]), "<>")
					keyValues = append(keyValues, KeyValue{
						Name:  keyName,
						Value: keyValue,
					})
				}
			}

			result[listName] = keyValues
		}
	}

	return result
}

// CmdGen provides a cobra command wrapping GenOptions
func CmdGen(streams genericiooptions.IOStreams) (*cobra.Command, error) {
	o := NewGenOptions(streams)

	cmd := &cobra.Command{
		Use:   "gen",
		Short: "Generate templates from YANG models",
		Long: `Generate configuration templates from YANG models.

This command takes a YANG model file and generates templates in various formats
(JSON, YAML, XML, SDC-CONF) for a specific path within the model.

Examples:
  # Generate JSON template for entire model
  kubectl sdcio gen --yang model.yang

  # Generate template for specific path
  kubectl sdcio gen --yang openconfig-interfaces.yang --path /interfaces/interface

  # Generate YAML template and save to file
  kubectl sdcio gen --yang model.yang --path /system/config --format yaml --output config.yaml`,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(c); err != nil {
				return err
			}
			return nil
		},
	}

	// Required flags
	cmd.Flags().StringVar(&o.yangPath, "yang", "", "Path to the YANG module file (required)")
	err := cmd.MarkFlagRequired("yang")
	if err != nil {
		return nil, err
	}

	// Optional flags
	cmd.Flags().StringVar(&o.modelPath, "path", "/", "Path in the YANG model (default: root)")
	cmd.Flags().StringVar(&o.outputFormat, "format", "json", "Output format: json, yaml, xml (default: json)")
	cmd.Flags().StringVarP(&o.outputFile, "output", "o", "", "Output file (default: stdout)")

	// Completion for formats
	if err := cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "yaml", "xml", "sdc-conf"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		return nil, err
	}

	// Completion for YANG files
	if err := cmd.RegisterFlagCompletionFunc("yang", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"yang"}, cobra.ShellCompDirectiveFilterFileExt
	}); err != nil {
		return nil, err
	}

	return cmd, nil
}

// remove keys attributes that are in the path
func (o *GenOptions) removeKeysFromTemplate(template map[string]interface{}, keys []string) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range template {
		// Skip if this key should be excluded
		if o.shouldExclude(k, keys) {
			continue
		}

		// Recursively process nested maps
		if nestedMap, ok := v.(map[string]interface{}); ok {
			result[k] = o.removeKeysFromTemplate(nestedMap, keys)
		} else {
			result[k] = v
		}
	}

	return result
}

// find keys in path
func (o *GenOptions) extractKeysFromPath(path string) []string {
	var keys []string
	// Match patterns like [name=<key>], [tac=<key>], etc.
	//re := regexp.MustCompile(`\[([^=]+)=<[^>]+>\]`)
	re := regexp.MustCompile(`([^=\[,]+)=<[^>]+>`)
	matches := re.FindAllStringSubmatch(path, -1)

	for _, match := range matches {
		if len(match) > 1 {
			keys = append(keys, match[1])
		}
	}
	return keys
}

// test attribute/key
func (o *GenOptions) shouldExclude(name string, excludeKeys []string) bool {
	for _, key := range excludeKeys {
		if name == key {
			return true
		}
	}
	return false
}
