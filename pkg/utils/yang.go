// pkg/utils/yang.go
package utils

import (
    "fmt"
    "path/filepath"
    "strings"
    "github.com/openconfig/goyang/pkg/yang"
)

type YangModuleInfo struct {
    Module    *yang.Module
    RootEntry *yang.Entry
    Namespace string
}

// LoadYangModule loads and processes a YANG file
func LoadYangModule(yangPath string) (*YangModuleInfo, error) {
    ms := yang.NewModules()
    
    if err := ms.Read(yangPath); err != nil {
        return nil, fmt.Errorf("failed to read YANG file: %v", err)
    }
    
    if errs := ms.Process(); len(errs) > 0 {
        var errMsgs []string
        for _, err := range errs {
            errMsgs = append(errMsgs, err.Error())
        }
        return nil, fmt.Errorf("failed to process YANG modules: %s", 
            strings.Join(errMsgs, "; "))
    }
    
    var mainModule *yang.Module
    for _, module := range ms.Modules {
        if module != nil {
            if strings.HasPrefix(filepath.Base(yangPath), module.Name) {
                mainModule = module
                break
            }
        }
    }
    
    if mainModule == nil {
        return nil, fmt.Errorf("no valid module found in YANG file")
    }
    
    rootEntry := yang.ToEntry(mainModule)
    if rootEntry == nil {
        return nil, fmt.Errorf("failed to convert module to entry")
    }
    
    namespace := ""
    if mainModule.Namespace != nil {
        namespace = mainModule.Namespace.Name
    }
    
    return &YangModuleInfo{
        Module:    mainModule,
        RootEntry: rootEntry,
        Namespace: namespace,
    }, nil
}
