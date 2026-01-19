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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"encoding/json"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

type SearchOptions struct {
	yangPath      string
	keyword       string
	outputFormat  string
	outputFile    string
	caseSensitive bool
	MyOptions
}

type SearchResult struct {
	Path        string
	LeafName    string
	Type        string
	Description string
}

// NewSearchOptions provides an instance of SearchOptions with default values
func NewSearchOptions(streams genericiooptions.IOStreams) *SearchOptions {
	return &SearchOptions{
		outputFormat:  "text",
		caseSensitive: false,
		MyOptions: MyOptions{
			IOStreams: streams,
		},
	}
}

func (o *SearchOptions) Complete(_ *cobra.Command, _ []string) error {
	// Check that the YANG file exists
	if o.yangPath != "" {
		if _, err := os.Stat(o.yangPath); os.IsNotExist(err) {
			return fmt.Errorf("YANG file '%s' does not exist", o.yangPath)
		}
	}
	return nil
}

// Validate validates the options
func (o *SearchOptions) Validate() error {
	if o.yangPath == "" {
		return fmt.Errorf("--yang parameter is required")
	}

	if o.keyword == "" {
		return fmt.Errorf("--yang-search parameter is required")
	}

	// Validate output format
	validFormats := []string{"text", "json", "yaml"}
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

func (o *SearchOptions) Run(_ *cobra.Command) error {
	// Search in YANG model
	results, moduleName, err := o.searchInYang()
	if err != nil {
		return fmt.Errorf("failed to search in YANG model: %v", err)
	}

	// Output the results
	return o.outputResults(results, moduleName)
}

func (o *SearchOptions) searchInYang() ([]SearchResult, string, error) {
	// Create a new module set
	ms := yang.NewModules()

	// Load the YANG file
	if err := ms.Read(o.yangPath); err != nil {
		return nil, "", fmt.Errorf("failed to read YANG file: %v", err)
	}

	// Process the modules
	if errs := ms.Process(); len(errs) > 0 {
		var errMsgs []string
		for _, err := range errs {
			errMsgs = append(errMsgs, err.Error())
		}
		return nil, "", fmt.Errorf("failed to process YANG modules: %s", strings.Join(errMsgs, "; "))
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
		return nil, "",fmt.Errorf("no valid module found in YANG file")
	}

	// Create root entry from module
	rootEntry := yang.ToEntry(mainModule)
	if rootEntry == nil {
		return nil, "", fmt.Errorf("failed to convert module to entry")
	}

	// Search for matching paths
	var results []SearchResult
	o.searchEntry(rootEntry, []string{}, &results)

	// Sort results by path
	sort.Slice(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})

	return results, mainModule.Name, nil
}

func (o *SearchOptions) searchEntry(entry *yang.Entry, currentPath []string, results *[]SearchResult) {
	if entry == nil {
		return
	}

	// Build current path
	var pathParts []string
	if len(currentPath) > 0 {
		pathParts = append(pathParts, currentPath...)
	}
	if entry.Name != "" {
		pathParts = append(pathParts, entry.Name)
	}
	fullPath := "/" + strings.Join(pathParts, "/")

	// Check if current entry matches the search criteria
	if o.matchesSearch(entry, fullPath) {
		result := SearchResult{
			Path:        fullPath,
			LeafName:    entry.Name,
			Type:        o.getEntryType(entry),
			Description: entry.Description,
		}
		*results = append(*results, result)
	}

	// Recursively search in children
	if entry.Dir != nil {
		for _, child := range entry.Dir {
			o.searchEntry(child, pathParts, results)
		}
	}
}

func (o *SearchOptions) matchesSearch(entry *yang.Entry, fullPath string) bool {
	// Check if leaf name matches
	leafMatches := o.matchesPattern(entry.Name, o.keyword)

	// Check if path matches
	pathMatches := o.matchesPattern(fullPath, o.keyword)

	// Check if description matches (if available)
	descMatches := false
	if entry.Description != "" {
		descMatches = o.matchesPattern(entry.Description, o.keyword)
	}

	return leafMatches || pathMatches || descMatches
}

func (o *SearchOptions) getEntryType(entry *yang.Entry) string {
	switch entry.Kind {
	case yang.DirectoryEntry:
		return "container"
	case yang.LeafEntry:
		if entry.ListAttr != nil {
			return "leaf-list"
		}
		if entry.Type != nil {
			return fmt.Sprintf("leaf (%s)", entry.Type.Name)
		}
		return "leaf"
	case yang.ChoiceEntry:
		return "choice"
	case yang.CaseEntry:
		return "case"
	case yang.AnyDataEntry:
		return "anydata"
	case yang.AnyXMLEntry:
		return "anyxml"
	case yang.NotificationEntry:
		return "notification"
	case yang.InputEntry:
		return "input"
	case yang.OutputEntry:
		return "output"
	default:
		if entry.ListAttr != nil {
			return "list"
		}
		return "unknown"
	}
}

// matchesPattern checks a string against a pattern (with wildcards)
// Reuses the same logic from client.go
func (o *SearchOptions) matchesPattern(text, pattern string) bool {
	if pattern == "" {
		return true
	}

	// Convert wildcard pattern to regex
	regexPattern := o.wildCardToRegexp(pattern)

	// Apply case sensitivity
	if !o.caseSensitive {
		text = strings.ToLower(text)
		pattern = strings.ToLower(pattern)
		regexPattern = "(?i)" + regexPattern
	}

	// Try regex match
	matched, err := regexp.MatchString(regexPattern, text)
	if err != nil {
		// Fallback to simple contains
		if !o.caseSensitive {
			return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
		}
		return strings.Contains(text, pattern)
	}
	return matched
}

// wildCardToRegexp converts pattern with wildcard to regex
// Reuses the same logic from client.go
func (o *SearchOptions) wildCardToRegexp(pattern string) string {
	// Escape any . in pattern
	pattern = strings.ReplaceAll(pattern, ".", "\\.")
	components := strings.Split(pattern, "*")
	if len(components) == 1 {
		// if len is 1, there are no *'s, return exact match pattern
		return "^" + pattern + "$"
	}
	var result strings.Builder
	for i, literal := range components {
		// Replace * with .*
		if i > 0 {
			result.WriteString(".*")
		}
		// Quote any regular expression meta characters in the literal text
		result.WriteString(regexp.QuoteMeta(literal))
	}
	return "^" + result.String() + "$"
}

// removeModulePrefix removes the module name from the beginning of the path
func (o *SearchOptions) removeModulePrefix(path string, moduleName string) string {
	if moduleName == "" {
		return path
	}

	// Remove leading slash for processing
	cleanPath := strings.TrimPrefix(path, "/")

	// Split the path
	parts := strings.Split(cleanPath, "/")

	// If the first part is the module name, remove it
	if len(parts) > 0 && parts[0] == moduleName {
		parts = parts[1:]
	}

	// Reconstruct the path
	if len(parts) == 0 {
		return "/"
	}
	return "/" + strings.Join(parts, "/")
}

func (o *SearchOptions) outputResults(results []SearchResult, moduleName string) error {
	if len(results) == 0 {
		fmt.Fprintf(o.Out, "No matches found for keyword: %s\n", o.keyword)
		return nil
	}

	// Remove module prefix from all result paths before output
	for i := range results {
		results[i].Path = o.removeModulePrefix(results[i].Path, moduleName)
	}

	var output string
	var err error

	switch strings.ToLower(o.outputFormat) {
	case "text":
		output = o.formatAsText(results)
	case "json":
		output, err = o.formatAsJSON(results)
		if err != nil {
			return err
		}
	case "yaml":
		output, err = o.formatAsYAML(results)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported format: %s", o.outputFormat)
	}

	// Write to file or stdout
	if o.outputFile != "" {
		return os.WriteFile(o.outputFile, []byte(output), 0644)
	}

	fmt.Fprint(o.Out, output)
	return nil
}

func (o *SearchOptions) formatAsText(results []SearchResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d match(es) for keyword: %s\n\n", len(results), o.keyword))

	for i, result := range results {
		sb.WriteString(fmt.Sprintf("%d. Path: %s\n", i+1, result.Path))
		sb.WriteString(fmt.Sprintf("   Leaf: %s\n", result.LeafName))
		sb.WriteString(fmt.Sprintf("   Type: %s\n", result.Type))
		if result.Description != "" {
			// Truncate long descriptions
			desc := result.Description
			if len(desc) > 100 {
				desc = desc[:97] + "..."
			}
			sb.WriteString(fmt.Sprintf("   Description: %s\n", desc))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (o *SearchOptions) formatAsJSON(results []SearchResult) (string, error) {

	output := map[string]interface{}{
		"keyword": o.keyword,
		"count":   len(results),
		"results": results,
	}

	bytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling JSON: %v", err)
	}
	return string(bytes), nil
}

func (o *SearchOptions) formatAsYAML(results []SearchResult) (string, error) {

	output := map[string]interface{}{
		"keyword": o.keyword,
		"count":   len(results),
		"results": results,
	}

	bytes, err := yaml.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("error marshaling YAML: %v", err)
	}
	return string(bytes), nil
}

// CmdSearch provides a cobra command wrapping SearchOptions
func CmdSearch(streams genericiooptions.IOStreams) (*cobra.Command, error) {
	o := NewSearchOptions(streams)

	cmd := &cobra.Command{
		Use:   "search-for",
		Short: "Search for keywords in YANG models",
		Long: `Search for keywords in YANG models and return matching paths.

This command takes a YANG model file and searches for a keyword (with wildcard support)
in leaf names, paths, and descriptions. It returns a list of matching YANG paths.

Wildcards:
  * - matches any sequence of characters
  
Examples:
  # Search for exact leaf name
  kubectl sdcio search-for --yang model.yang --yang-search ambulance

  # Search with wildcard
  kubectl sdcio search-for --yang model.yang --yang-search "*timeout*"

  # Search case-sensitive
  kubectl sdcio search-for --yang model.yang --yang-search "Interface" --case-sensitive

  # Output as JSON
  kubectl sdcio search-for --yang model.yang --yang-search "*config*" --format json

  # Save results to file
  kubectl sdcio search-for --yang model.yang --yang-search "*ip*" --output results.txt`,
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

	cmd.Flags().StringVar(&o.keyword, "yang-search", "", "Keyword to search for (supports * wildcard) (required)")
	err = cmd.MarkFlagRequired("yang-search")
	if err != nil {
		return nil, err
	}

	// Optional flags
	cmd.Flags().StringVar(&o.outputFormat, "format", "text", "Output format: text, json, yaml (default: text)")
	cmd.Flags().StringVarP(&o.outputFile, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().BoolVar(&o.caseSensitive, "case-sensitive", false, "Enable case-sensitive search (default: false)")

	// Completion for formats
	if err := cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"text", "json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
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
