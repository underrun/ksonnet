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

package clicmd

import (
	"fmt"

	"github.com/ksonnet/ksonnet/pkg/actions"
	"github.com/ksonnet/ksonnet/pkg/app"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	vEnvSetName      = "env-set-name"
	vEnvSetNamespace = "env-set-namespace"
	vEnvSetServer    = "env-set-server"
	vEnvSetAPISpec   = "env-set-spec-flag"
)

var (
	envSetLong = `
The ` + "`set`" + ` command lets you change the fields of an existing environment.
You can currently only update your environment's name.

Note that changing the name of an environment will also update the corresponding
directory structure in ` + "`environments/`" + `.

### Related Commands

* ` + "`ks env list` " + `— ` + envShortDesc["list"] + `

### Syntax
`
	envSetExample = `#Update the name of the environment 'us-west/staging'.
# Updating the name will update the directory structure in 'environments/'.
ks env set us-west/staging --name=us-east/staging`
)

func newEnvSetCmd(a app.App) *cobra.Command {
	envSetCmd := &cobra.Command{
		Use:     "set <env-name>",
		Short:   envShortDesc["set"],
		Long:    envSetLong,
		Example: envSetExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("'env set' takes a single argument, that is the name of the environment")
			}

			m := map[string]interface{}{
				actions.OptionApp:        a,
				actions.OptionEnvName:    args[0],
				actions.OptionNewEnvName: viper.GetString(vEnvSetName),
				actions.OptionNamespace:  viper.GetString(vEnvSetNamespace),
				actions.OptionServer:     viper.GetString(vEnvSetServer),
				actions.OptionSpecFlag:   viper.GetString(vEnvSetAPISpec),
			}

			return runAction(actionEnvSet, m)
		},
	}

	envSetCmd.Flags().String(flagEnvName, "",
		"Name used to uniquely identify the environment. Must not already exist within the ksonnet app")
	viper.BindPFlag(vEnvSetName, envSetCmd.Flags().Lookup(flagName))

	envSetCmd.Flags().String(flagNamespace, "",
		"Namespace for environment")
	viper.BindPFlag(vEnvSetNamespace, envSetCmd.Flags().Lookup(flagNamespace))

	envSetCmd.Flags().String(flagServer, "",
		"Cluster server for environment")
	viper.BindPFlag(vEnvSetServer, envSetCmd.Flags().Lookup(flagServer))

	envSetCmd.Flags().String(flagAPISpec, "",
		"Kubernetes version for environment")
	viper.BindPFlag(vEnvSetAPISpec, envSetCmd.Flags().Lookup(flagAPISpec))
	return envSetCmd
}
