#!/usr/bin/env bash

export GOPATH=${HOME}/go
export PATH=${PATH}:/usr/local/go/bin:${GOPATH}/bin

sudo systemctl stop slurm-agent
go build -o bin/slurm-agent cmd/slurm-agent/slurm-agent.go
cp bin/slurm-agent /usr/local/bin/slurm-agent

sudo mkdir -p /var/run/slurm-agent

mkdir -p /var/run/slurm-agent
mkdir -p /var/run/slurm-bridge-operator/
chown -R slurm:slurm /var/run/slurm-bridge-operator/

sudo sh -c 'cat  > /etc/systemd/system/slurm-agent.service <<EOF
[Unit]
Description=Slurm bridge operator slurm-agent
StartLimitIntervalSec=0

[Service]
Type=simple
Restart=always
RestartSec=30
User=root
Group=root
WorkingDirectory=/var/run/slurm-agent
ExecStart=/usr/local/bin/slurm-agent
EOF'

sudo systemctl daemon-reload
sudo systemctl start slurm-agent
sudo systemctl status slurm-agent
sudo systemctl enable slurm-agent
