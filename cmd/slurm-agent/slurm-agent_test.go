package main

import (
	"github.com/sirupsen/logrus"
	"testing"
)

func Test_slurmAgent(t *testing.T) {
	logrus.Info("Sbatch: ", "#/bin/bash \n", "Args: ", "--n ss")

}
