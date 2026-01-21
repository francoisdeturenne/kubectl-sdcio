// pkg/utils/path.go
package utils

import (
    "regexp"
    "strings"
)

// StripKeysFromPath removes key expressions like [name=<key>]
func StripKeysFromPath(path string) string {
    re := regexp.MustCompile(`\[[^\]]+=<[^>]+>\]`)
    return re.ReplaceAllString(path, "")
}

// RemoveModulePrefix removes the module name from path
func RemoveModulePrefix(path string, moduleName string) string {
    if moduleName == "" {
        return path
    }
    
    cleanPath := strings.TrimPrefix(path, "/")
    parts := strings.Split(cleanPath, "/")
    
    if len(parts) > 0 {
        firstPart := parts[0]
        if idx := strings.Index(firstPart, "["); idx != -1 {
            firstPart = firstPart[:idx]
        }
        
        if firstPart == moduleName {
            parts = parts[1:]
        }
    }
    
    if len(parts) == 0 {
        return "/"
    }
    return "/" + strings.Join(parts, "/")
}

type KeyValue struct {
    Name  string
    Value string
}

// ExtractKeysFromPath extracts key names from path
func ExtractKeysFromPath(path string) []string {
    var keys []string
    re := regexp.MustCompile(`([^=\[,]+)=<[^>]+>`)
    matches := re.FindAllStringSubmatch(path, -1)
    
    for _, match := range matches {
        if len(match) > 1 {
            keys = append(keys, match[1])
        }
    }
    return keys
}

// ExtractPathKeysWithValues extracts keys with their values
func ExtractPathKeysWithValues(path string) map[string][]KeyValue {
    result := make(map[string][]KeyValue)
    
    re := regexp.MustCompile(`([^/\[]+)\[([^\]]+)\]`)
    matches := re.FindAllStringSubmatch(path, -1)
    
    for _, match := range matches {
        if len(match) > 2 {
            listName := match[1]
            keysStr := match[2]
            
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
