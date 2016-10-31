// Copyright © 2016 Dylan Clendenin <dylan@betterdoctor.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/betterdoctor/duncan/marathon"
	"github.com/spf13/cobra"
)

// scaleCmd represents the scale command
var scaleCmd = &cobra.Command{
	Use:   "scale",
	Short: "Scale an app process",
	Long: `Scale processes within an application group by name and count

Examples:

duncan scale web=2 --app foo --env production
duncan scale web=2 worker=5 --app foo --env production

If application cannot scale due to insufficient cluster resources an error will be returned
	`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			printUsageAndExit()
		}
		if env != "" && env != "stage" && env != "production" {
			fmt.Printf("env %s is not a valid deployment environment\n", env)
			os.Exit(-1)
		}
		// validate args match proc=count format
		for _, p := range args {
			s := strings.Split(p, "=")
			if len(s) != 2 {
				printUsageAndExit()
			}
			_, err := strconv.Atoi(s[1])
			if err != nil {
				printUsageAndExit()
			}
		}
		if err := marathon.Scale(app, env, args); err != nil {
			fmt.Println(err)
			os.Exit(-1)
		}
	},
}

func init() {
	RootCmd.AddCommand(scaleCmd)

	scaleCmd.Flags().StringVarP(&app, "app", "a", "", "optionally filter by app")
	scaleCmd.Flags().StringVarP(&env, "env", "e", "", "optionally filter by environment (stage, production)")
}

func printUsageAndExit() {
	fmt.Println("USAGE: duncan scale web=3 [worker=2, ...] --app foo --env production")
	os.Exit(-1)
}