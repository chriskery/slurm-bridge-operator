// Copyright (c) 2019 Sylabs, Inc. All rights reserved.
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

package main

import (
	"context"
	"flag"
	kubeclusterorgv1alpha1 "github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/pkg/configurator"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"os"
	"os/signal"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sync"
	"time"
)

var (
	serviceAccount = os.Getenv("SERVICE_ACCOUNT")
	kubeletImage   = os.Getenv("KUBELET_IMAGE")
	resultsImage   = os.Getenv("RESULTS_IMAGE")
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kubeclusterorgv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var probeAddr string
	var controllerThreads int
	var updateInterval time.Duration
	var agentHttpEndpoint string
	var agentSock string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.IntVar(&controllerThreads, "controller-threads", 1, "Number of worker threads used by the controller.")
	flag.DurationVar(&updateInterval, "update-interval", 30*time.Second, "how often configurator checks state")
	flag.StringVar(&agentHttpEndpoint, "endpoint", ":9999", "http endpoint for slurm agent")
	flag.StringVar(&agentSock, "sock", "", "path to slurm agent socket")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
	})

	go func() {
		setupLog.Info("starting manager")
		if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			setupLog.Error(err, "problem running manager")
			os.Exit(1)
		}
	}()

	addr := agentSock
	if len(addr) == 0 {
		addr = agentHttpEndpoint
	} else {
		addr = "unix://" + agentSock
	}

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logrus.Fatalf("can't connect to %s %s", agentSock, err)
	}
	slurmC := workload.NewWorkloadManagerClient(conn)

	cf := configurator.NewConfigurator(mgr, slurmC, addr)
	cf.InitVKServiceAccount()

	ctx, cancel := context.WithCancel(context.Background())

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go cf.WatchPartitions(ctx, wg, updateInterval)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, unix.SIGINT, unix.SIGTERM, unix.SIGQUIT)

	logrus.Infof("Got signal %s", <-sig)
	cancel()

	wg.Wait()

	logrus.Info("Configurator is finished")
}
