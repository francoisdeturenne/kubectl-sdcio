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

import "strings"

// DependencyInfo contains all dependency information for a YANG node
type DependencyInfo struct {
	// Keys this node depends on (from parent lists)
	RequiredKeys []KeyDependency `json:"required_keys,omitempty" yaml:"required_keys,omitempty"`

	// Leafrefs pointing to other paths
	References []Reference `json:"references,omitempty" yaml:"references,omitempty"`

	// Nodes that reference this path
	ReferencedBy []string `json:"referenced_by,omitempty" yaml:"referenced_by,omitempty"`

	// Must/when conditions
	Constraints []Constraint `json:"constraints,omitempty" yaml:"constraints,omitempty"`
}

// KeyDependency represents a dependency on a list key
type KeyDependency struct {
	ListPath string   `json:"list_path" yaml:"list_path"`
	KeyNames []string `json:"key_names" yaml:"key_names"`
	Level    int      `json:"level" yaml:"level"` // Distance from current node
}

// Reference represents a reference to another YANG path
type Reference struct {
	Type        string `json:"type" yaml:"type"` // "leafref", "instance-identifier", etc.
	TargetPath  string `json:"target_path" yaml:"target_path"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Constraint represents a must or when condition
type Constraint struct {
	Type       string `json:"type" yaml:"type"` // "must", "when"
	Expression string `json:"expression" yaml:"expression"`
	ErrorMsg   string `json:"error_message,omitempty" yaml:"error_message,omitempty"`
}

// PathContext holds context information during tree traversal
type PathContext struct {
	pathParts []string
	listKeys  []KeyDependency
}

// NewPathContext creates a new PathContext
func NewPathContext() *PathContext {
	return &PathContext{
		pathParts: []string{},
		listKeys:  []KeyDependency{},
	}
}

// Clone creates a deep copy of PathContext
func (pc *PathContext) Clone() *PathContext {
	newContext := &PathContext{
		pathParts: make([]string, len(pc.pathParts)),
		listKeys:  make([]KeyDependency, len(pc.listKeys)),
	}
	copy(newContext.pathParts, pc.pathParts)
	copy(newContext.listKeys, pc.listKeys)
	return newContext
}

// AddPathPart adds a new path segment
func (pc *PathContext) AddPathPart(part string) {
	pc.pathParts = append(pc.pathParts, part)
}

// AddListKey adds a new list key dependency
func (pc *PathContext) AddListKey(listPath string, keyNames []string) {
	pc.listKeys = append(pc.listKeys, KeyDependency{
		ListPath: listPath,
		KeyNames: keyNames,
		Level:    0,
	})
}

// IncrementKeyLevels increases the level of all key dependencies
func (pc *PathContext) IncrementKeyLevels() {
	for i := range pc.listKeys {
		pc.listKeys[i].Level++
	}
}

// GetFullPath returns the complete path as a string
func (pc *PathContext) GetFullPath() string {
	if len(pc.pathParts) == 0 {
		return "/"
	}
	return "/" + strings.Join(pc.pathParts, "/")
}
