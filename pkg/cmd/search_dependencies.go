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
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
)

// DependencyCollector handles collection of dependency information
type DependencyCollector struct {
	reverseRefIndex map[string][]string
	options         *SearchOptions
}

// NewDependencyCollector creates a new dependency collector
func NewDependencyCollector(options *SearchOptions) *DependencyCollector {
	return &DependencyCollector{
		reverseRefIndex: make(map[string][]string),
		options:         options,
	}
}

// BuildReverseReferenceIndex builds an index of reverse references
func (dc *DependencyCollector) BuildReverseReferenceIndex(rootEntry *yang.Entry) {
	dc.indexReferences(rootEntry, []string{})
}

// indexReferences recursively indexes all references in the tree
func (dc *DependencyCollector) indexReferences(entry *yang.Entry, currentPath []string) {
	if entry == nil {
		return
	}

	pathParts := currentPath
	if entry.Name != "" {
		pathParts = append(pathParts, entry.Name)
	}
	fullPath := "/" + strings.Join(pathParts, "/")

	// Check for leafrefs in the entry's type
	if entry.Type != nil {
		dc.indexTypeReferences(entry.Type, fullPath)
	}

	// Recurse into children
	if entry.Dir != nil {
		for _, child := range entry.Dir {
			dc.indexReferences(child, pathParts)
		}
	}
}

// indexTypeReferences indexes references found in a YangType
func (dc *DependencyCollector) indexTypeReferences(yangType *yang.YangType, sourcePath string) {
	if yangType == nil {
		return
	}

	// Check for leafref type
	if yangType.Kind == yang.Yleafref && yangType.Path != "" {
		targetPath := dc.normalizeXPath(yangType.Path)
		dc.reverseRefIndex[targetPath] = append(dc.reverseRefIndex[targetPath], sourcePath)
	}

	// Check union types for nested leafrefs
	if yangType.Kind == yang.Yunion && len(yangType.Type) > 0 {
		for _, unionType := range yangType.Type {
			dc.indexTypeReferences(unionType, sourcePath)
		}
	}
}

// CollectDependencies collects all dependency information for an entry
func (dc *DependencyCollector) CollectDependencies(entry *yang.Entry, context *PathContext) *DependencyInfo {
	deps := &DependencyInfo{}

	// 1. Collect required keys from parent lists
	if len(context.listKeys) > 0 {
		deps.RequiredKeys = make([]KeyDependency, len(context.listKeys))
		copy(deps.RequiredKeys, context.listKeys)
	}

	// 2. Collect leafref and other references
	deps.References = dc.collectReferences(entry)

	// 3. Collect constraints (must/when from Node if available)
	deps.Constraints = dc.collectConstraints(entry)

	// Return nil if no dependencies found
	if len(deps.RequiredKeys) == 0 &&
		len(deps.References) == 0 &&
		len(deps.Constraints) == 0 {
		return nil
	}

	return deps
}

// collectReferences collects all reference types (leafref, instance-identifier, etc.)
func (dc *DependencyCollector) collectReferences(entry *yang.Entry) []Reference {
	var refs []Reference

	if entry.Type == nil {
		return refs
	}

	refs = dc.collectTypeReferences(entry.Type)
	return refs
}

// collectTypeReferences recursively collects references from a YangType
func (dc *DependencyCollector) collectTypeReferences(yangType *yang.YangType) []Reference {
	var refs []Reference

	if yangType == nil {
		return refs
	}

	// Check for leafref type
	if yangType.Kind == yang.Yleafref {
		ref := Reference{
			Type:       "leafref",
			TargetPath: dc.normalizeXPath(yangType.Path),
		}
		refs = append(refs, ref)
	}

	// Check for instance-identifier
	if yangType.Kind == yang.YinstanceIdentifier {
		ref := Reference{
			Type:       "instance-identifier",
			TargetPath: "dynamic", // Instance-identifier is runtime
		}
		if yangType.OptionalInstance {
			ref.Description = "optional instance"
		} else {
			ref.Description = "required instance"
		}
		refs = append(refs, ref)
	}

	// Check union types for nested leafrefs
	if yangType.Kind == yang.Yunion && len(yangType.Type) > 0 {
		for _, unionType := range yangType.Type {
			unionRefs := dc.collectTypeReferences(unionType)
			refs = append(refs, unionRefs...)
		}
	}

	return refs
}

// collectConstraints collects must and when statements from the entry's Node
func (dc *DependencyCollector) collectConstraints(entry *yang.Entry) []Constraint {
	var constraints []Constraint

	// Check if entry has a Node (the underlying YANG statement)
	if entry.Node == nil {
		return constraints
	}

	// Look for must statements in the Node's statements
	for _, stmt := range entry.Node.Statement().SubStatements() {
		switch stmt.Keyword {
		case "must":
			constraint := Constraint{
				Type:       "must",
				Expression: stmt.Argument,
			}
			// Look for error-message sub-statement
			for _, subStmt := range stmt.SubStatements() {
				if subStmt.Keyword == "error-message" {
					constraint.ErrorMsg = subStmt.Argument
					break
				}
			}
			constraints = append(constraints, constraint)

		case "when":
			constraint := Constraint{
				Type:       "when",
				Expression: stmt.Argument,
			}
			constraints = append(constraints, constraint)
		}
	}

	return constraints
}

// PopulateReferencedBy adds reverse reference information to results
func (dc *DependencyCollector) PopulateReferencedBy(results []SearchResult) {
	for i := range results {
		if refs, exists := dc.reverseRefIndex[results[i].Path]; exists {
			if results[i].Dependencies == nil {
				results[i].Dependencies = &DependencyInfo{}
			}
			results[i].Dependencies.ReferencedBy = refs
		}
	}
}

// normalizeXPath normalizes an XPath expression to a simple path
func (dc *DependencyCollector) normalizeXPath(xpath string) string {
	// Remove leading/trailing whitespace
	xpath = strings.TrimSpace(xpath)

	// Handle relative paths (../)
	if strings.HasPrefix(xpath, "../") {
		// For now, just return as-is
		// A full implementation would resolve relative paths
		return xpath
	}

	// Remove any predicates [...]
	result := strings.Builder{}
	inPredicate := false
	for _, ch := range xpath {
		if ch == '[' {
			inPredicate = true
		} else if ch == ']' {
			inPredicate = false
		} else if !inPredicate {
			result.WriteRune(ch)
		}
	}

	return result.String()
}

// FormatDependencies formats dependency information for text output
func (dc *DependencyCollector) FormatDependencies(deps *DependencyInfo) string {
	if deps == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("   Dependencies:\n")

	// Required keys
	if len(deps.RequiredKeys) > 0 {
		sb.WriteString("     Required Keys:\n")
		for _, keyDep := range deps.RequiredKeys {
			sb.WriteString(fmt.Sprintf("       - %s: %s (level %d)\n",
				keyDep.ListPath,
				strings.Join(keyDep.KeyNames, ", "),
				keyDep.Level))
		}
	}

	// References
	if len(deps.References) > 0 {
		sb.WriteString("     References:\n")
		for _, ref := range deps.References {
			refStr := fmt.Sprintf("       - %s -> %s", ref.Type, ref.TargetPath)
			if ref.Description != "" {
				refStr += fmt.Sprintf(" (%s)", ref.Description)
			}
			sb.WriteString(refStr + "\n")
		}
	}

	// Referenced by
	if len(deps.ReferencedBy) > 0 {
		sb.WriteString("     Referenced By:\n")
		for _, refBy := range deps.ReferencedBy {
			sb.WriteString(fmt.Sprintf("       - %s\n", refBy))
		}
	}

	// Constraints
	if len(deps.Constraints) > 0 {
		sb.WriteString("     Constraints:\n")
		for _, constraint := range deps.Constraints {
			sb.WriteString(fmt.Sprintf("       - %s: %s\n",
				constraint.Type,
				constraint.Expression))
			if constraint.ErrorMsg != "" {
				sb.WriteString(fmt.Sprintf("         Error: %s\n", constraint.ErrorMsg))
			}
		}
	}

	return sb.String()
}
