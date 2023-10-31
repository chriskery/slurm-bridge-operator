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

package v1alpha1

import (
	"errors"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// ErrAffinityIsNotRequired signalise that affinity for requested resources is not required.
var ErrAffinityIsNotRequired = errors.New("affinity selectors is not required")
var DefaultNodeSelectors = map[string]string{
	"type": "virtual-kubelet",
}
var PartitionLabel = "kubecluster.org/partition"
var DefaultTolerations = []corev1.Toleration{
	{
		Key:      "virtual-kubelet.io/provider",
		Operator: corev1.TolerationOpEqual,
		Value:    "slurm-agent",
		Effect:   corev1.TaintEffectNoSchedule,
	},
}

// Resources describes job resources which will be transformed into k8s pod affinity.
type Resources struct {
	Nodes      int64
	MemPerNode int64
	CPUPerNode int64
	WallTime   time.Duration
}

// AffinityForResources returns k8s affinity for requested resources
// In case empty(default) resources ErrAffinityIsNotRequired will be returned.
func AffinityForResources(r Resources) (*corev1.Affinity, error) {
	var nodeMatch []corev1.NodeSelectorRequirement
	if r.Nodes != 0 {
		nodeMatch = append(nodeMatch, corev1.NodeSelectorRequirement{
			Key:      "sbj.kubecluster.org/nodes",
			Operator: "Gt",
			Values:   []string{strconv.FormatInt(r.Nodes-1, 10)},
		})
	}
	if r.WallTime != 0 {
		nodeMatch = append(nodeMatch, corev1.NodeSelectorRequirement{
			Key:      "sbj.kubecluster.org/wall-time",
			Operator: "Gt",
			Values:   []string{strconv.FormatInt(int64(r.WallTime/time.Second)-1, 10)},
		})
	}
	if r.MemPerNode != 0 {
		nodeMatch = append(nodeMatch, corev1.NodeSelectorRequirement{
			Key:      "sbj.kubecluster.org/mem-per-node",
			Operator: "Gt",
			Values:   []string{strconv.FormatInt(r.MemPerNode-1, 10)},
		})
	}
	if r.CPUPerNode != 0 {
		nodeMatch = append(nodeMatch, corev1.NodeSelectorRequirement{
			Key:      "sbj.kubecluster.org/cpu-per-node",
			Operator: "Gt",
			Values:   []string{strconv.FormatInt(r.CPUPerNode-1, 10)},
		})
	}

	if len(nodeMatch) == 0 {
		return nil, ErrAffinityIsNotRequired
	}

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: nodeMatch}},
			},
		},
	}, nil
}
