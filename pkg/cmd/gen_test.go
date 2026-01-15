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
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

func TestGenCommand_NokiaCMMCallTrace(t *testing.T) {
	// Skip test if YANG file doesn't exist
	yangFile := "/Users/efde6331/stu/nf-sim-integrated/devsim/yang/nokia-cmm-24.7/nokia-cn-cmm-conf.yang"
	if _, err := os.Stat(yangFile); os.IsNotExist(err) {
		t.Skipf("YANG file not found: %s", yangFile)
	}

	tests := []struct {
		name         string
		path         string
		format       string
		expectError  bool
		validateFunc func(t *testing.T, output string)
	}{
		{
			name:   "generate JSON template for call-trace setting",
			path:   "/configure/shared/call-trace/setting",
			format: "json",
			validateFunc: func(t *testing.T, output string) {
				// Validate JSON structure
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err, "Output should be valid JSON")

				// Check if the output contains expected structure
				assert.NotEmpty(t, result, "Result should not be empty")

				// Log the output for manual inspection
				t.Logf("Generated template:\n%s", output)
			},
		},
		{
			name:   "generate YAML template for call-trace setting",
			path:   "/configure/shared/call-trace/setting",
			format: "yaml",
			validateFunc: func(t *testing.T, output string) {
				// Basic validation that output is not empty and contains YAML-like structure
				assert.NotEmpty(t, output, "YAML output should not be empty")
				assert.Contains(t, output, ":", "YAML should contain key-value pairs")

				// Log the output for manual inspection
				t.Logf("Generated YAML template:\n%s", output)
			},
		},
		{
			name:   "generate XML template for call-trace setting",
			path:   "/configure/shared/call-trace/setting",
			format: "xml",
			validateFunc: func(t *testing.T, output string) {
				// Basic validation for XML structure
				assert.NotEmpty(t, output, "XML output should not be empty")

				// Log the output for manual inspection
				t.Logf("Generated XML template:\n%s", output)
			},
		},
		{
			name:   "generate template for root path",
			path:   "/",
			format: "json",
			validateFunc: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err, "Root template should be valid JSON")

				// Should contain configure section
				assert.Contains(t, result, "configure", "Root should contain configure section")

				t.Logf("Root template keys: %v", getKeys(result))
			},
		},
		{
			name:        "invalid path should return error",
			path:        "/invalid/nonexistent/path",
			format:      "json",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			errBuf := &bytes.Buffer{}
			streams := genericiooptions.IOStreams{
				In:     &bytes.Buffer{},
				Out:    buf,
				ErrOut: errBuf,
			}

			cmd, err := CmdGen(streams)
			require.NoError(t, err)

			args := []string{
				"--yang", yangFile,
				"--path", tt.path,
				"--format", tt.format,
			}
			cmd.SetArgs(args)

			err = cmd.Execute()

			if tt.expectError {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
				t.Logf("Expected error: %v", err)
			} else {
				assert.NoError(t, err, "Unexpected error for test case: %s", tt.name)
				if err != nil {
					t.Logf("Error output: %s", errBuf.String())
				}

				if tt.validateFunc != nil {
					tt.validateFunc(t, buf.String())
				}
			}
		})
	}
}

func TestGenCommand_OutputToFile(t *testing.T) {
	yangFile := "/Users/efde6331/stu/nf-sim-integrated/devsim/yang/nokia-cmm-24.7/nokia-cn-cmm-conf.yang"
	if _, err := os.Stat(yangFile); os.IsNotExist(err) {
		t.Skipf("YANG file not found: %s", yangFile)
	}

	// Create temporary directory for output file
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "call-trace-template.json")

	buf := &bytes.Buffer{}
	streams := genericiooptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    buf,
		ErrOut: &bytes.Buffer{},
	}

	cmd, err := CmdGen(streams)
	require.NoError(t, err)

	args := []string{
		"--yang", yangFile,
		"--path", "/configure/shared/call-trace/setting",
		"--format", "json",
		"--output", outputFile,
	}
	cmd.SetArgs(args)

	err = cmd.Execute()
	require.NoError(t, err)

	// Verify file was created
	assert.FileExists(t, outputFile)

	// Verify file content
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(content, &result)
	require.NoError(t, err, "Output file should contain valid JSON")

	t.Logf("Output file created: %s", outputFile)
	t.Logf("File content preview: %s", string(content)[:min(200, len(content))])
}

func TestGenCommand_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedErr string
	}{
		{
			name:        "missing yang parameter",
			args:        []string{"--path", "/test"},
			expectedErr: "--yang parameter is required",
		},
		{
			name:        "invalid format",
			args:        []string{"--yang", "test.yang", "--format", "invalid"},
			expectedErr: "invalid output format",
		},
		{
			name:        "nonexistent yang file",
			args:        []string{"--yang", "nonexistent.yang"},
			expectedErr: "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			streams := genericiooptions.IOStreams{
				In:     &bytes.Buffer{},
				Out:    buf,
				ErrOut: &bytes.Buffer{},
			}

			cmd, err := CmdGen(streams)
			require.NoError(t, err)

			cmd.SetArgs(tt.args)
			err = cmd.Execute()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

// Helper function to get keys from a map
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
