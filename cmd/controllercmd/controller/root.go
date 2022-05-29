/*
Copyright 2022 NDD.

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

package controller

import (
	"os"

	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	//certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	//certmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	//admissionv1 "k8s.io/api/admissionregistration/v1"
	//extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	debug    bool
	profiler bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "manager",
	Short: "ndd srl config controller",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.SilenceUsage = true
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug mode")
	rootCmd.PersistentFlags().BoolVarP(&profiler, "profiler", "", false, "enable profiling")

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(targetv1.AddToScheme(scheme))
	utilruntime.Must(pkgmetav1.AddToScheme(scheme))
	utilruntime.Must(pkgv1.AddToScheme(scheme))
	//utilruntime.Must(certv1.AddToScheme(scheme))
	//utilruntime.Must(certmetav1.AddToScheme(scheme))
	//utilruntime.Must(admissionv1.AddToScheme(scheme))
	//utilruntime.Must(extv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}
