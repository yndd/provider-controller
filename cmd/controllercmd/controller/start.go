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
	"context"
	"os"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/pkg/errors"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"

	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/provider-controller/internal/controllers"
	"github.com/yndd/registrator/registrator"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/ratelimiter"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	//+kubebuilder:scaffold:imports
)

var (
	metricsAddr               string
	probeAddr                 string
	enableLeaderElection      bool
	concurrency               int
	pollInterval              time.Duration
	namespace                 string
	podname                   string
	grpcServerAddress         string
	grpcQueryAddress          string
	autoPilot                 bool
	revision                  string
	revisionNamespace         string
	serviceDiscoveryNamespace string   // todo initialization
	controllerConfigName      string   // To be removed
	crdNames                  []string // to be removed
)

// startCmd represents the start command for the network device driver
var startCmd = &cobra.Command{
	Use:          "start",
	Short:        "start the srl ndd config controller",
	Long:         "start the srl ndd config controller",
	Aliases:      []string{"start"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		zlog := zap.New(zap.UseDevMode(debug), zap.JSONEncoder())
		if debug {
			// Only use a logr.Logger when debug is on
			ctrl.SetLogger(zlog)
		}
		logger := logging.NewLogrLogger(zlog.WithName("provider-controller"))

		if profiler {
			defer profile.Start().Stop()
			go func() {
				http.ListenAndServe(":8000", nil)
			}()
		}

		zlog.Info("create manager")
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     metricsAddr,
			Port:                   7443,
			HealthProbeBindAddress: probeAddr,
			LeaderElection:         enableLeaderElection,
			LeaderElectionID:       "c66ce353.ndd.yndd.io",
		})
		if err != nil {
			return errors.Wrap(err, "Cannot create manager")
		}

		// get k8s client
		client, err := getClient(scheme)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		zlog.Info("serviceDiscoveryNamespace", "serviceDiscoveryNamespace", serviceDiscoveryNamespace)
		reg := registrator.NewConsulRegistrator(ctx, serviceDiscoveryNamespace, "",
			registrator.WithClient(resource.ClientApplicator{
				Client:     client,
				Applicator: resource.NewAPIPatchingApplicator(client),
			}),
			registrator.WithLogger(logger))

		nddcopts := &shared.NddControllerOptions{
			Logger:            logger,
			Registrator:       reg,
			Poll:              pollInterval,
			Namespace:         namespace,
			Revision:          revision,
			RevisionNamespace: revisionNamespace,
			Copts: controller.Options{
				MaxConcurrentReconciles: concurrency,
				RateLimiter:             ratelimiter.NewDefaultProviderRateLimiter(ratelimiter.DefaultProviderRPS),
			},
		}

		// initialize controllers
		if err = controllers.Setup(mgr, nddcopts); err != nil {
			return errors.Wrap(err, "Cannot add ndd controllers to manager")
		}

		// +kubebuilder:scaffold:builder

		if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
			return errors.Wrap(err, "unable to set up health check")
		}
		if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
			return errors.Wrap(err, "unable to set up ready check")
		}

		zlog.Info("starting manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			return errors.Wrap(err, "problem running manager")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVarP(&metricsAddr, "metrics-bind-address", "m", ":8080", "The address the metric endpoint binds to.")
	startCmd.Flags().StringVarP(&probeAddr, "health-probe-bind-address", "p", ":8081", "The address the probe endpoint binds to.")
	startCmd.Flags().BoolVarP(&enableLeaderElection, "leader-elect", "l", false, "Enable leader election for controller manager. "+
		"Enabling this will ensure there is only one active controller manager.")
	startCmd.Flags().IntVarP(&concurrency, "concurrency", "", 1, "Number of items to process simultaneously")
	startCmd.Flags().DurationVarP(&pollInterval, "poll-interval", "", 10*time.Minute, "Poll interval controls how often an individual resource should be checked for drift.")
	startCmd.Flags().StringVarP(&namespace, "namespace", "n", os.Getenv("POD_NAMESPACE"), "Namespace used to unpack and run packages.")
	startCmd.Flags().StringVarP(&podname, "podname", "", os.Getenv("POD_NAME"), "Name from the pod")
	startCmd.Flags().StringVarP(&grpcServerAddress, "grpc-server-address", "s", "", "The address of the grpc server binds to.")
	startCmd.Flags().StringVarP(&grpcQueryAddress, "grpc-query-address", "", "", "Validation query address.")
	startCmd.Flags().BoolVarP(&autoPilot, "autopilot", "a", true,
		"Apply delta/diff changes to the config automatically when set to true, if set to false the provider will report the delta and the operator should intervene what to do with the delta/diffs")
	startCmd.Flags().StringVarP(&revision, "revision", "", "", "The name of the provider revision that holds the status ifnormation for the controller")
	startCmd.Flags().StringVarP(&revisionNamespace, "revision-namespace", "", "", "The namespace of the provider revision that holds the status ifnormation for the controller")
	startCmd.Flags().StringVarP(&serviceDiscoveryNamespace, "service-discovery-namespace", "", "consul", "the namespace for service discovery")
	startCmd.Flags().StringVarP(&controllerConfigName, "controller-config-name", "", "", "The name of the controller configuration")
	startCmd.Flags().StringSliceVarP(&crdNames, "crd-names", "", []string{}, "The name of the crds")
}

func getClient(scheme *runtime.Scheme) (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}
