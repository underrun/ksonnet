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

package actions

import (
	"testing"

	"github.com/ksonnet/ksonnet/pkg/app"
	amocks "github.com/ksonnet/ksonnet/pkg/app/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvSet(t *testing.T) {
	envName := "old_env_name"
	newName := "new_env_name"
	oldNamespace := "old_namespace"
	namespace := "new_namesapce"
	oldServer := "old_server"
	server := "new_server"
	newk8sAPISpec := "version:new_api_spec"

	environmentMockFn := func(name string) *app.EnvironmentSpec {
		return &app.EnvironmentSpec{
			Destination: &app.EnvironmentDestinationSpec{
				Namespace: oldNamespace,
				Server:    oldServer,
			},
		}
	}

	withApp(t, func(appMock *amocks.App) {
		cases := []struct {
			name        string
			in          map[string]interface{}
			spec        *app.EnvironmentSpec
			envRenameFn func(t *testing.T) envRenameFn
			updateEnvFn func(t *testing.T) updateEnvFn
		}{
			{
				name: "rename environment",
				in: map[string]interface{}{
					OptionApp:        appMock,
					OptionEnvName:    envName,
					OptionNewEnvName: newName,
				},
				envRenameFn: func(t *testing.T) envRenameFn {
					return func(a app.App, from, to string, override bool) error {
						assert.Equal(t, envName, from)
						assert.Equal(t, newName, to)
						assert.False(t, override)

						return nil
					}
				},
			},
			{
				name: "set new namespace",
				in: map[string]interface{}{
					OptionApp:       appMock,
					OptionEnvName:   envName,
					OptionNamespace: namespace,
				},
				updateEnvFn: func(t *testing.T) updateEnvFn {
					return func(a app.App, envName, k8sAPISpec string, spec *app.EnvironmentSpec, override bool) error {
						assert.Equal(t, spec, &app.EnvironmentSpec{
							Destination: &app.EnvironmentDestinationSpec{
								Namespace: namespace,
								Server:    oldServer,
							},
						})
						return nil
					}
				},
			},
			{
				name: "set new server",
				in: map[string]interface{}{
					OptionApp:     appMock,
					OptionEnvName: envName,
					OptionServer:  server,
				},
				updateEnvFn: func(t *testing.T) updateEnvFn {
					return func(a app.App, envName, k8sAPISpec string, spec *app.EnvironmentSpec, override bool) error {
						assert.Equal(t, spec, &app.EnvironmentSpec{
							Destination: &app.EnvironmentDestinationSpec{
								Namespace: oldNamespace,
								Server:    server,
							},
						})
						return nil
					}
				},
			},
			{
				name: "set new api spec",
				in: map[string]interface{}{
					OptionApp:      appMock,
					OptionEnvName:  envName,
					OptionSpecFlag: newk8sAPISpec,
				},
				updateEnvFn: func(t *testing.T) updateEnvFn {
					return func(a app.App, envName, k8sAPISpec string, spec *app.EnvironmentSpec, override bool) error {
						assert.Equal(t, newk8sAPISpec, k8sAPISpec)
						return nil
					}
				},
			},
			{
				name: "set everything at once",
				in: map[string]interface{}{
					OptionApp:        appMock,
					OptionEnvName:    envName,
					OptionNewEnvName: newName,
					OptionNamespace:  namespace,
					OptionServer:     server,
					OptionSpecFlag:   newk8sAPISpec,
				},
				updateEnvFn: func(t *testing.T) updateEnvFn {
					return func(a app.App, newName, k8sAPISpec string, spec *app.EnvironmentSpec, override bool) error {
						assert.Equal(t, spec, &app.EnvironmentSpec{
							Destination: &app.EnvironmentDestinationSpec{
								Namespace: namespace,
								Server:    server,
							},
						})
						assert.Equal(t, newk8sAPISpec, k8sAPISpec)
						return nil
					}
				},
				envRenameFn: func(t *testing.T) envRenameFn {
					return func(a app.App, from, to string, override bool) error {
						assert.Equal(t, envName, from)
						assert.Equal(t, newName, to)
						assert.False(t, override)

						return nil
					}
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				a, err := NewEnvSet(tc.in)
				require.NoError(t, err)

				if tc.envRenameFn != nil {
					a.envRenameFn = tc.envRenameFn(t)
				}

				if tc.updateEnvFn != nil {
					a.updateEnvFn = tc.updateEnvFn(t)
				}

				appMock.On("Environment", tc.in[OptionEnvName]).Return(environmentMockFn, nil)

				err = a.Run()
				require.NoError(t, err)

			})
		}
	})
}

func TestEnvSet_requires_app(t *testing.T) {
	in := make(map[string]interface{})
	_, err := NewEnvSet(in)
	require.Error(t, err)
}
