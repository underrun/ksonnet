// Copyright 2018 The ksonnet authors
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package component

import (
	"path/filepath"
	"strings"

	"github.com/google/go-jsonnet/ast"
	"github.com/ksonnet/ksonnet/pkg/app"
	"github.com/ksonnet/ksonnet/pkg/schema"
	jsonnetutil "github.com/ksonnet/ksonnet/pkg/util/jsonnet"
	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
)

// ParamOptions is options for parameters.
type ParamOptions struct {
	Index int
}

// Summary summarizes items found in components.
type Summary struct {
	ComponentName string `json:"component_name,omitempty"`
	Type          string `json:"type,omitempty"`
	APIVersion    string `json:"api_version,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Name          string `json:"name,omitempty"`
}

// GVK converts a summary to a group - version - kind.
func (s *Summary) typeSpec() (*schema.TypeSpec, error) {
	return schema.NewTypeSpec(s.APIVersion, s.Kind)
}

// Component is a ksonnet Component interface.
type Component interface {
	// DeleteParam deletes a component parameter.
	DeleteParam(path []string) error
	// Name is the component name.
	Name(wantsNamedSpaced bool) string
	// Params returns a list of all parameters for a component. If envName is a
	// blank string, it will report the local parameters.
	Params(envName string) ([]ModuleParameter, error)
	// Remove removes the component
	Remove() error
	// SetParams sets a component paramaters.
	SetParam(path []string, value interface{}) error
	// Summarize returns a summary of the component.
	Summarize() (Summary, error)
	// ToNode converts a component to a Jsonnet node.
	ToNode(envName string) (string, ast.Node, error)
	// Type returns the type of component.
	Type() string
}

const (
	// componentsDir is the name of the directory which houses components.
	componentsRoot = "components"
	// paramsFile is the params file for a component namespace.
	paramsFile = "params.libsonnet"
)

// LocateComponent locates a component given a module and a name.
func LocateComponent(ksApp app.App, module, name string) (Component, error) {
	path := make([]string, 0)
	if module != "" && module != "/" {
		path = append(path, module)
	}

	path = append(path, name)
	return ExtractComponent(ksApp, strings.Join(path, "/"))
}

// Path returns returns the file system path for a component.
func Path(a app.App, name string) (string, error) {
	ns, localName := extractModuleComponent(a, name)

	fis, err := afero.ReadDir(a.Fs(), ns.Dir())
	if err != nil {
		return "", err
	}

	var fileName string
	files := make(map[string]bool)

	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}

		base := strings.TrimSuffix(fi.Name(), filepath.Ext(fi.Name()))
		if _, ok := files[base]; ok {
			return "", errors.Errorf("Found multiple component files with component name %q", name)
		}
		files[base] = true

		if base == localName {
			fileName = fi.Name()
		}
	}

	if fileName == "" {
		return "", errors.Errorf("No component name %q found", name)
	}

	return filepath.Join(ns.Dir(), fileName), nil
}

// ExtractComponent extracts a component from a path.
func ExtractComponent(a app.App, path string) (Component, error) {
	ns, componentName := extractModuleComponent(a, path)
	members, err := ns.Components()
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		if componentName == member.Name(false) {
			return member, nil
		}
	}

	return nil, errors.Errorf("unable to find component %q", componentName)
}

func isComponentDir(fs afero.Fs, path string) (bool, error) {
	files, err := afero.ReadDir(fs, path)
	if err != nil {
		return false, errors.Wrapf(err, "read files in %s", path)
	}

	for _, file := range files {
		if file.Name() == paramsFile {
			return true, nil
		}
	}

	return false, nil
}

// MakePaths creates a slice of component paths
func MakePaths(a app.App, env string) ([]string, error) {
	cpl, err := newComponentPathLocator(a, env)
	if err != nil {
		return nil, errors.Wrap(err, "create component path locator")
	}

	return cpl.Locate()
}

func envParams(a app.App, moduleName, envName string) (string, error) {
	libPath, err := a.LibPath(envName)
	if err != nil {
		return "", err
	}

	ns, err := GetModule(a, moduleName)
	if err != nil {
		return "", err
	}

	_, paramsStr, err := ns.ResolvedParams(envName)
	if err != nil {
		return "", err
	}

	data, err := a.EnvironmentParams(envName)
	if err != nil {
		return "", err
	}

	env, err := a.Environment(envName)
	if err != nil {
		return "", err
	}

	envParams := upgradeParams(envName, data)

	vm := jsonnetutil.NewVM()
	vm.AddJPath(
		libPath,
		env.MakePath(a.Root()),
		filepath.Join(a.Root(), "lib"),
		filepath.Join(a.Root(), "vendor"),
	)
	vm.ExtCode("__ksonnet/params", paramsStr)
	log.Debugf("[component.go:envParams] Evaluating: %v", envParams)
	return vm.EvaluateSnippet("snippet", string(envParams))
}
