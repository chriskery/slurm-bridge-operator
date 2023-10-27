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

package api

import (
	"context"
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/pkg/errors"
)

const localFilePrefix = "local.file"

type (
	// Slurm implements WorkloadManagerServer.
	Slurm struct {
		uid    int64
		cfg    Config
		client *slurm.Client
	}

	// Config is a red-box configuration for each partition available.
	Config map[string]PartitionResources

	// PartitionResources configure how red-box will see slurm partition resources.
	// In auto mode red-box will attempt to query partition resources from slurm, but
	// administrator can set up them manually.
	PartitionResources struct {
		AutoNodes      bool `yaml:"auto_nodes"`
		AutoCPUPerNode bool `yaml:"auto_cpu_per_node"`
		AutoMemPerNode bool `yaml:"auto_mem_per_node"`
		AutoWallTime   bool `yaml:"auto_wall_time"`

		Nodes      int64         `yaml:"nodes"`
		CPUPerNode int64         `yaml:"cpu_per_node"`
		MemPerNode int64         `yaml:"mem_per_node"`
		WallTime   time.Duration `yaml:"wall_time"`

		AdditionalFeatures []Feature `yaml:"additional_features"`
	}

	// Feature represents slurm partition feature.
	Feature struct {
		Name     string `yaml:"name"`
		Version  string `yaml:"version"`
		Quantity int64  `yaml:"quantity"`
	}
)

// NewSlurm creates a new instance of Slurm.
func NewSlurm(c *slurm.Client, cfg Config) *Slurm {
	return &Slurm{client: c, cfg: cfg, uid: int64(os.Geteuid())}
}

// SubmitJob submits job and returns id of it in case of success.
func (s *Slurm) SubmitJob(ctx context.Context, req *workload.SubmitJobRequest) (*workload.SubmitJobResponse, error) {
	// todo use client id from req
	id, err := s.client.SBatch(req.Script, req.Partition)
	if err != nil {
		return nil, errors.Wrap(err, "could not submit sbatch script")
	}

	return &workload.SubmitJobResponse{
		JobId: id,
	}, nil
}

// SubmitJobContainer starts a container from the provided image name inside a sbatch script.
func (s *Slurm) SubmitJobContainer(ctx context.Context, r *workload.SubmitJobContainerRequest) (*workload.SubmitJobContainerResponse, error) {
	script := buildSLURMScript(r)

	id, err := s.client.SBatch(script, r.Partition)
	if err != nil {
		return nil, errors.Wrap(err, "could not submit sbatch script")
	}

	return &workload.SubmitJobContainerResponse{
		JobId: id,
	}, nil
}

// CancelJob cancels job.
func (s *Slurm) CancelJob(ctx context.Context, req *workload.CancelJobRequest) (*workload.CancelJobResponse, error) {
	if err := s.client.SCancel(req.JobId); err != nil {
		return nil, errors.Wrapf(err, "could not cancel job %d", req.JobId)
	}

	return &workload.CancelJobResponse{}, nil
}

// JobInfo returns information about a job from 'scontrol show jobid'.
// Safe to call before job finished. After it could return an error.
func (s *Slurm) JobInfo(ctx context.Context, req *workload.JobInfoRequest) (*workload.JobInfoResponse, error) {
	info, err := s.client.SJobInfo(req.JobId)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get job %d info", req.JobId)
	}

	pInfo, err := mapSInfoToProtoInfo(info)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert slurm info into proto info")
	}

	if len(pInfo) == 0 {
		return nil, errors.New("job info slice is empty, probably invalid scontrol output")
	}

	return &workload.JobInfoResponse{Info: pInfo}, nil
}

// JobSteps returns information about job steps from 'sacct'.
// Safe to call after job started. Before it could return an error.
func (s *Slurm) JobSteps(ctx context.Context, req *workload.JobStepsRequest) (*workload.JobStepsResponse, error) {
	steps, err := s.client.SJobSteps(req.JobId)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get job %d steps", req.JobId)
	}

	pSteps, err := toProtoSteps(steps)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert slurm steps into proto steps")
	}

	return &workload.JobStepsResponse{JobSteps: pSteps}, nil
}

// OpenFile opens requested file and return chunks with bytes.
func (s *Slurm) OpenFile(r *workload.OpenFileRequest, req workload.WorkloadManager_OpenFileServer) error {
	fd, err := s.client.Open(r.Path)
	if err != nil {
		return errors.Wrapf(err, "could not open file at %s", r.Path)
	}
	defer fd.Close()

	buff := make([]byte, 128)
	for {
		n, err := fd.Read(buff)
		if n > 0 {
			if err := req.Send(&workload.Chunk{Content: buff[:n]}); err != nil {
				return errors.Wrap(err, "could not send chunk")
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}

			return err
		}
	}

	return nil
}

// TailFile tails a file till close requested.
// To start receiving file bytes client should send a request with file path and action start,
// to stop client should send a request with action readToEndAndClose (file path is not required)
// and after reaching end method will send EOF error.
func (s *Slurm) TailFile(req workload.WorkloadManager_TailFileServer) error {
	r, err := req.Recv()
	if err != nil {
		return errors.Wrap(err, "could not receive request")
	}

	fd, err := s.client.Tail(r.Path)
	if err != nil {
		return errors.Wrapf(err, "could not tail file at %s", r.Path)
	}
	defer func(p string) {
		log.Printf("Tail file at %s finished", p)
	}(r.Path)

	requestCh := make(chan *workload.TailFileRequest)
	go func() {
		r, err := req.Recv()
		if err != nil {
			if err != io.EOF {
				log.Printf("could not recive request err: %s", err)
			}
			return
		}

		requestCh <- r
	}()

	buff := make([]byte, 128)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-req.Context().Done():
			return req.Context().Err()
		case r := <-requestCh:
			if r.Action == workload.TailAction_ReadToEndAndClose {
				_ = fd.Close()
			}
		case <-ticker.C:
			n, err := fd.Read(buff)
			if err != nil && n == 0 {
				return err
			}

			if n == 0 {
				continue
			}

			if err := req.Send(&workload.Chunk{Content: buff[:n]}); err != nil {
				return errors.Wrap(err, "could not send chunk")
			}
		}
	}
}

// Resources return available resources on slurm cluster in a requested partition.
func (s *Slurm) Resources(_ context.Context, req *workload.ResourcesRequest) (*workload.ResourcesResponse, error) {
	slurmResources, err := s.client.Resources(req.Partition)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get resources for partition %s", req.Partition)
	}

	partitionResources := s.cfg[req.Partition]
	response := &workload.ResourcesResponse{
		Nodes:      partitionResources.Nodes,
		CpuPerNode: partitionResources.CPUPerNode,
		MemPerNode: partitionResources.MemPerNode,
		WallTime:   int64(partitionResources.WallTime.Seconds()),
	}

	for _, f := range slurmResources.Features {
		response.Features = append(response.Features, &workload.Feature{
			Name:     f.Name,
			Version:  f.Version,
			Quantity: f.Quantity,
		})
	}
	for _, f := range partitionResources.AdditionalFeatures {
		response.Features = append(response.Features, &workload.Feature{
			Name:     f.Name,
			Version:  f.Version,
			Quantity: f.Quantity,
		})
	}

	if partitionResources.AutoNodes || response.Nodes == 0 {
		response.Nodes = slurmResources.Nodes
	}
	if partitionResources.AutoCPUPerNode || response.CpuPerNode == 0 {
		response.CpuPerNode = slurmResources.CPUPerNode
	}
	if partitionResources.AutoMemPerNode || response.MemPerNode == 0 {
		response.MemPerNode = slurmResources.MemPerNode
	}
	if partitionResources.AutoWallTime || response.WallTime == 0 {
		response.WallTime = int64(slurmResources.WallTime.Seconds())
	}

	return response, nil
}

// Partitions returns partition names.
func (s *Slurm) Partitions(context.Context, *workload.PartitionsRequest) (*workload.PartitionsResponse, error) {
	names, err := s.client.Partitions()
	if err != nil {
		return nil, errors.Wrap(err, "could not get partition names")
	}

	return &workload.PartitionsResponse{Partition: names}, nil
}

// WorkloadInfo returns wlm info (name, version, red-box uid)
func (s *Slurm) WorkloadInfo(context.Context, *workload.WorkloadInfoRequest) (*workload.WorkloadInfoResponse, error) {
	const wlmName = "slurm"

	sVersion, err := s.client.Version()
	if err != nil {
		return nil, errors.Wrap(err, "could not get slurm version")
	}

	return &workload.WorkloadInfoResponse{
		Name:    wlmName,
		Version: sVersion,
		Uid:     s.uid,
	}, nil
}

func toProtoSteps(ss []*slurm.JobStepInfo) ([]*workload.JobStepInfo, error) {
	pSteps := make([]*workload.JobStepInfo, len(ss))

	for i, s := range ss {
		var startedAt *timestamp.Timestamp
		if s.StartedAt != nil {
			pt, err := ptypes.TimestampProto(*s.StartedAt)
			if err != nil {
				return nil, errors.Wrap(err, "could not convert started go time to proto time")
			}

			startedAt = pt
		}

		var finishedAt *timestamp.Timestamp
		if s.FinishedAt != nil {
			pt, err := ptypes.TimestampProto(*s.FinishedAt)
			if err != nil {
				return nil, errors.Wrap(err, "could not convert finished go time to proto time")
			}

			finishedAt = pt
		}

		status, ok := workload.JobStatus_value[s.State]
		if !ok {
			status = int32(workload.JobStatus_UNKNOWN)
		}

		pSteps[i] = &workload.JobStepInfo{
			Id:        s.ID,
			Name:      s.Name,
			ExitCode:  int32(s.ExitCode),
			Status:    workload.JobStatus(status),
			StartTime: startedAt,
			EndTime:   finishedAt,
		}
	}

	return pSteps, nil
}

func mapSInfoToProtoInfo(si []*slurm.JobInfo) ([]*workload.JobInfo, error) {
	pInfs := make([]*workload.JobInfo, len(si))
	for i, inf := range si {
		var submitTime *timestamp.Timestamp
		if inf.SubmitTime != nil {
			pt, err := ptypes.TimestampProto(*inf.SubmitTime)
			if err != nil {
				return nil, errors.Wrap(err, "could not convert submit go time to proto time")
			}

			submitTime = pt
		}

		var startTime *timestamp.Timestamp
		if inf.StartTime != nil {
			pt, err := ptypes.TimestampProto(*inf.StartTime)
			if err != nil {
				return nil, errors.Wrap(err, "could not convert start go time to proto time")
			}

			startTime = pt
		}

		var runTime *duration.Duration
		if inf.RunTime != nil {
			runTime = ptypes.DurationProto(*inf.RunTime)
		}

		var timeLimit *duration.Duration
		if inf.TimeLimit != nil {
			timeLimit = ptypes.DurationProto(*inf.TimeLimit)
		}

		status, ok := workload.JobStatus_value[inf.State]
		if !ok {
			status = int32(workload.JobStatus_UNKNOWN)
		}

		pi := workload.JobInfo{
			Id:         inf.ID,
			UserId:     inf.UserID,
			Name:       inf.Name,
			ExitCode:   inf.ExitCode,
			Status:     workload.JobStatus(status),
			SubmitTime: submitTime,
			StartTime:  startTime,
			RunTime:    runTime,
			TimeLimit:  timeLimit,
			WorkingDir: inf.WorkDir,
			StdOut:     inf.StdOut,
			StdErr:     inf.StdErr,
			Partition:  inf.Partition,
			NodeList:   inf.NodeList,
			BatchHost:  inf.BatchHost,
			NumNodes:   inf.NumNodes,
			ArrayId:    inf.ArrayJobID,
		}
		pInfs[i] = &pi
	}

	return pInfs, nil
}

func buildSLURMScript(r *workload.SubmitJobContainerRequest) string {
	const (
		verifyT = `srun singularity verify "%s" || exit`
		rmT     = `srun rm "%s"`

		timeT       = `#SBATCH --time=0:%d` //seconds
		memT        = `#SBATCH --mem=%d`    //mbs
		nodesT      = `#SBATCH --nodes=%d`
		cpuPerTaskT = `#SBATCH --cpus-per-task=%d`
	)

	runT := buildRunCommand(r.Options)

	pullT := `srun singularity pull --name "%s" "%s" || exit` // secure pull
	if r.Options.AllowUnsigned {
		pullT = `srun singularity pull -U --name "%s" "%s" || exit` // unsecured pull
	}

	lines := []string{"#!/bin/sh"}

	if r.WallTime != 0 {
		lines = append(lines, fmt.Sprintf(timeT, r.WallTime))
	}

	if r.MemPerNode != 0 {
		lines = append(lines, fmt.Sprintf(memT, r.MemPerNode))
	}

	if r.Nodes != 0 {
		lines = append(lines, fmt.Sprintf(nodesT, r.Nodes))
	}

	if r.CpuPerNode != 0 {
		lines = append(lines, fmt.Sprintf(cpuPerTaskT, r.CpuPerNode))
	}

	// checks if sif is located somewhere on the host machine
	if strings.HasPrefix(r.ImageName, localFilePrefix) {
		image := strings.TrimPrefix(r.ImageName, localFilePrefix)
		if !r.Options.AllowUnsigned {
			lines = append(lines, fmt.Sprintf(verifyT, image))
		}
		lines = append(lines, fmt.Sprintf(runT, image))
	} else {
		id := uuid.New().String()
		lines = append(lines, fmt.Sprintf(pullT, id, r.ImageName))
		lines = append(lines, fmt.Sprintf(runT, id))
		lines = append(lines, fmt.Sprintf(rmT, id))
	}

	return strings.Join(lines, "\n")
}

func buildRunCommand(opt *workload.SingularityOptions) string {
	run := "srun singularity run"
	flags := []string{}

	if opt.App != "" {
		flags = append(flags, fmt.Sprintf(`--app="%s"`, opt.App))
	}
	if opt.HostName != "" {
		flags = append(flags, fmt.Sprintf(`--hostname="%s"`, opt.HostName))
	}

	if len(opt.Binds) != 0 {
		bind := strings.Join(opt.Binds, ",")
		flags = append(flags, fmt.Sprintf(`--bind="%s"`, bind))
	}

	if opt.ClearEnv {
		flags = append(flags, "-c")
	}
	if opt.FakeRoot {
		flags = append(flags, "-f")
	}
	if opt.Ipc {
		flags = append(flags, "-i")
	}
	if opt.Pid {
		flags = append(flags, "-p")
	}
	if opt.NoPrivs {
		flags = append(flags, "--no-privs")
	}
	if opt.Writable {
		flags = append(flags, "-w")
	}

	if len(flags) != 0 {
		run = fmt.Sprintf("%s %s", run, strings.Join(flags, " "))
	}
	return run + " " + `"%s" || exit`
}
