package main

import (
	"github.com/chriskery/slurm-bridge-operator/cmd/slurm-virtual-kubelet/app"
	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"github.com/virtual-kubelet/virtual-kubelet/trace/opencensus"
	"k8s.io/component-base/cli"
	"os"
)

func init() {
	log.L = logruslogger.FromLogrus(logrus.NewEntry(logrus.StandardLogger()))
	trace.T = opencensus.Adapter{}
}

func main() {
	command := app.NewSlurmVirtualKubeltCommand()
	code := cli.Run(command)
	os.Exit(code)
}
