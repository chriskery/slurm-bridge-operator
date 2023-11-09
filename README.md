# Slurm Bridge Operator

Slurm bridge operator is a Kubernetes operator implementation, capable of submitting and monitoring slurm jobs, It acts
as a proxy for the external slurm cluster in the k8s cluster

## Description

The slurm bridge operator consists of the following five components:

- [SlurmAgent](cmd/slurm-agent/slurm-agent.go)
- [Configurator](cmd/configurator/configurator.go)
- [SlurmVirtualKubelet](cmd/slurm-virtual-kubelet/slurm-virtual-kubelet.go)
- [BridgeOperator](cmd/bridge-operator/bridge-operator.go)
- [ResultFetcher](cmd/result-fetcher/result-fetcher.go)

## Getting Started

You’ll need a Kubernetes cluster and a Slurm cluster to run against.

### Running on the cluster

1. Clone the repo.

```bash
git clone https://github.com/chriskery/slurm-bridge-operator
cd slurm-bridge-operator
```

2. Install Slurm Bridge Operator CRDs.

```sh
maek install
###Build and push your image to the location specified by `IMG`.
make docker-build docker-push IMG=<some-registry>/slurm-agent-bridge-operator:tag
###Deploy the controller to the cluster with the image specified by `IMG`.
make deploy IMG=<some-registry>/slurm-agent-bridge-operator:tag
```

3. Build and install slurm agent on the slurm login node as a proxy between kubernetes and slurm cluster.
   The slurm-agent binary file will be build to bin/slurm-agent

```shell
make build

```

4. Install configurator.

```shell
kubectl apply -f manifests/configurator.yaml
```

## Quick Start

Please refer to the [quick-start.md](docs/quick-start.md)  for more information.

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

