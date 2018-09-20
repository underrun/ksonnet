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

package pipeline

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"regexp"
	gostrings "strings"

	log "github.com/sirupsen/logrus"

	"github.com/ksonnet/ksonnet-lib/ksonnet-gen/astext"
	"github.com/ksonnet/ksonnet-lib/ksonnet-gen/printer"
	"github.com/ksonnet/ksonnet/pkg/app"
	"github.com/ksonnet/ksonnet/pkg/component"
	"github.com/ksonnet/ksonnet/pkg/env"
	clustermetadata "github.com/ksonnet/ksonnet/pkg/metadata"
	"github.com/ksonnet/ksonnet/pkg/params"
	"github.com/ksonnet/ksonnet/pkg/util/jsonnet"
	"github.com/ksonnet/ksonnet/pkg/util/k8s"
	"github.com/ksonnet/ksonnet/pkg/util/strings"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// OverrideManager overrides the component manager interface for a pipeline.
func OverrideManager(c component.Manager) Opt {
	return func(p *Pipeline) {
		p.cm = c
	}
}

// Opt is an option for configuring Pipeline.
type Opt func(p *Pipeline)

// Pipeline is the ks build pipeline.
type Pipeline struct {
	app                 app.App
	envName             string
	cm                  component.Manager
	buildObjectsFn      func(*Pipeline, []string) ([]*unstructured.Unstructured, error)
	evaluateEnvFn       func(a app.App, envName, components, paramsStr string, opts ...jsonnet.VMOpt) (string, error)
	evaluateEnvParamsFn func(a app.App, sourcePath, paramsStr, envName, moduleName string) (string, error)
	stubModuleFn        func(m component.Module) (string, error)
}

// New creates an instance of Pipeline.
func New(ksApp app.App, envName string, opts ...Opt) *Pipeline {
	log.Debugf("creating ks pipeline for environment %q", envName)
	p := &Pipeline{
		app:                 ksApp,
		envName:             envName,
		cm:                  component.DefaultManager,
		buildObjectsFn:      buildObjects,
		evaluateEnvFn:       env.Evaluate,
		evaluateEnvParamsFn: params.EvaluateEnv,
		stubModuleFn:        stubModule,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Modules returns the modules that belong to this pipeline.
func (p *Pipeline) Modules() ([]component.Module, error) {
	return p.cm.Modules(p.app, p.envName)
}

// EnvParameters creates parameters for a namespace given an environment.
func (p *Pipeline) EnvParameters(moduleName string, inherited bool) (string, error) {
	module, err := p.cm.Module(p.app, moduleName)
	if err != nil {
		return "", errors.Wrapf(err, "load module %s", moduleName)
	}

	paramsStr, err := p.moduleParams(module, inherited)
	if err != nil {
		return "", err
	}

	data, err := p.app.EnvironmentParams(p.envName)
	if err != nil {
		return "", errors.Wrapf(err, "retrieve environment params for %s", p.envName)
	}

	envParams := upgradeParams(p.envName, data)

	env, err := p.app.Environment(p.envName)
	if err != nil {
		return "", errors.Wrapf(err, "load environment %s", p.envName)
	}

	vm := jsonnet.NewVM()
	vm.AddJPath(
		env.MakePath(p.app.Root()),
		filepath.Join(p.app.Root(), "lib"),
		filepath.Join(p.app.Root(), "vendor"),
	)
	vm.ExtCode("__ksonnet/params", paramsStr)
	log.Debugf("[Pipeline.EnvParameters] Evaluating: %v", envParams)
	return vm.EvaluateSnippet("snippet", string(envParams))
}

func (p *Pipeline) moduleParams(module component.Module, inherited bool) (string, error) {
	if !inherited {
		return stubModule(module)
	}

	_, paramsStr, err := module.ResolvedParams(p.envName)
	if err != nil {
		return "", errors.Wrapf(err, "resolve params for %s", module.Name())
	}

	return paramsStr, nil
}

func stubModule(module component.Module) (string, error) {
	componentsObject := map[string]interface{}{}

	components, err := module.Components()
	if err != nil {
		return "", errors.Wrap(err, "loading module components")
	}

	for _, c := range components {
		componentsObject[c.Name(true)] = make(map[string]interface{})
	}

	m := map[string]interface{}{
		"components": componentsObject,
	}

	data, err := json.Marshal(&m)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Components returns the components that belong to this pipeline.
func (p *Pipeline) Components(filter []string) ([]component.Component, error) {
	modules, err := p.Modules()
	if err != nil {
		return nil, err
	}

	components := make([]component.Component, 0)
	for _, m := range modules {
		members, err := p.cm.Components(p.app, m.Name())
		if err != nil {
			return nil, err
		}

		members = filterComponents(filter, members)
		components = append(components, members...)
	}

	return components, nil
}

// Objects converts components into Kubernetes objects.
func (p *Pipeline) Objects(filter []string) ([]*unstructured.Unstructured, error) {
	return p.buildObjectsFn(p, filter)
}

func (p *Pipeline) moduleObjects(module component.Module, filter []string) ([]*unstructured.Unstructured, error) {
	doc := &astext.Object{}

	object, componentMap, err := module.Render(p.envName, filter...)
	if err != nil {
		return nil, err
	}

	doc.Fields = append(doc.Fields, object.Fields...)

	// apply environment parameters
	noGlobalParamData, moduleParamData, err := module.ResolvedParams(p.envName)
	if err != nil {
		return nil, err
	}

	envParamsPath, err := env.Path(p.app, p.envName, "params.libsonnet")
	if err != nil {
		return nil, err
	}

	envParamData, err := p.evaluateEnvParamsFn(p.app, envParamsPath, moduleParamData, p.envName, module.Name())
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err = printer.Fprint(&buf, doc); err != nil {
		return nil, err
	}

	// evaluate module with jsonnet.
	evaluated, err := p.evaluateEnvFn(p.app, p.envName, buf.String(), envParamData)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}

	if err = json.Unmarshal([]byte(evaluated), &m); err != nil {
		return nil, err
	}

	ret := make([]runtime.Object, 0, len(m))

	for componentName, v := range m {
		if len(filter) != 0 && !strings.InSlice(componentName, filter) {
			continue
		}

		componentObject, ok := v.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("component %q is not an object", componentName)
		}

		labelComponents(componentObject, componentName)

		data, err := json.Marshal(componentObject)
		if err != nil {
			return nil, err
		}

		componentType, ok := componentMap[componentName]
		if !ok {
			// Items in a list won't end up in this map, so assume they are jsonnet.
			componentType = "jsonnet"
		}

		var patched string

		switch componentType {
		case "jsonnet":
			patched = string(data)
		case "yaml":
			patched, err = params.PatchJSON(string(data), noGlobalParamData, componentName)
			if err != nil {
				return nil, errors.Wrap(err, "patching YAML/JSON component")
			}
		}

		uns, _, err := unstructured.UnstructuredJSONScheme.Decode([]byte(patched), nil, nil)
		if err != nil {
			return nil, errors.Wrap(err, "decoding unstructured")
		}
		ret = append(ret, uns)
	}

	return k8s.FlattenToV1(ret)
}

// YAML converts components into YAML.
func (p *Pipeline) YAML(filter []string) (io.Reader, error) {
	objects, err := p.Objects(filter)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := Fprint(&buf, objects, "yaml"); err != nil {
		return nil, errors.Wrap(err, "convert objects to YAML")
	}

	return &buf, nil
}

func filterComponents(filter []string, components []component.Component) []component.Component {
	if len(filter) == 0 {
		return components
	}

	var out []component.Component
	for _, c := range components {
		if strings.InSlice(c.Name(true), filter) {
			out = append(out, c)
		}
	}

	return out
}

var (
	reParamSwap = regexp.MustCompile(`(?m)import "\.\.\/\.\.\/components\/params\.libsonnet"`)
)

// upgradeParams replaces relative params imports with an extVar to handle
// multiple component namespaces.
// NOTE: It warns when it makes a change. This serves as a temporary fix until
// ksonnet generates the correct file.
func upgradeParams(envName, in string) string {
	if reParamSwap.MatchString(in) {
		log.Warnf("rewriting %q environment params to not use relative paths", envName)
		return reParamSwap.ReplaceAllLiteralString(in, `std.extVar("__ksonnet/params")`)
	}

	return in
}

func buildObjects(p *Pipeline, filter []string) ([]*unstructured.Unstructured, error) {
	modules, err := p.Modules()
	if err != nil {
		return nil, errors.Wrap(err, "get modules")
	}

	var ret []*unstructured.Unstructured

	for _, m := range modules {
		log.WithFields(log.Fields{
			"action":      "pipeline",
			"module-name": m.Name(),
		}).Debug("building objects")

		objects, err := p.moduleObjects(m, filter)
		if err != nil {
			return nil, err
		}

		ret = append(ret, objects...)
	}

	return ret, nil
}

func labelComponents(m map[string]interface{}, name string) {
	if m["apiVersion"] == "v1" && m["kind"] == "List" {
		list, ok := m["items"].([]interface{})
		if !ok {
			return
		}

		for _, item := range list {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			labelComponent(itemMap, name)
		}

		return
	}

	labelComponent(m, name)
}

func labelComponent(m map[string]interface{}, name string) {
	metadata, ok := m["metadata"].(map[string]interface{})
	if !ok {
		metadata = make(map[string]interface{})
		m["metadata"] = metadata
	}

	labels, ok := metadata["labels"].(map[string]interface{})
	if !ok {
		labels = make(map[string]interface{})
		metadata["labels"] = labels
	}

	// TODO: this should be owned by module
	name = gostrings.TrimPrefix(name, "/")
	name = gostrings.Replace(name, "/", ".", -1)

	labels[clustermetadata.LabelComponent] = name
}
