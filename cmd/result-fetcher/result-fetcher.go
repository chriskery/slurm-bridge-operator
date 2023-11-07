package main

import (
	"context"
	"flag"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"

	"google.golang.org/grpc"
)

var (
	from = flag.String("from", "", "specify file name to collect")
	to   = flag.String("to", "", "specify directory where to put file")
)

func main() {
	var agentHttpEndpoint string
	var agentSock string
	flag.StringVar(&agentHttpEndpoint, "endpoint", ":9999", "http endpoint for slurm-agent agent")
	flag.StringVar(&agentSock, "sock", "", "path to slurm-agent agent socket")
	flag.Parse()

	if *from == "" {
		logrus.Fatalf("from can't be empty")
	}

	if *to == "" {
		logrus.Fatalf("to can't be empty")
	}

	if agentSock == "" && agentHttpEndpoint == "" {
		logrus.Fatalf("slurm agent endpoint can't be empty")
	}

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
	client := workload.NewWorkloadManagerClient(conn)

	openReq, err := client.OpenFile(context.Background(), &workload.OpenFileRequest{Path: *from})
	if err != nil {
		log.Fatalf("can't open file err: %s", err)
	}

	if err = os.MkdirAll(*to, 0755); err != nil {
		logrus.Fatalf("can't create dir on mounted volume err: %s", err)
	}

	filePath := path.Join(*to, filepath.Base(*from))
	toFile, err := os.Create(filePath)
	if err != nil {
		logrus.Fatalf("can't not create file with results on mounted volume err: %s", err)
	}
	defer toFile.Close()

	for {
		chunk, err := openReq.Recv()

		if chunk != nil {
			if _, err := toFile.Write(chunk.Content); err != nil {
				log.Fatalf("can't write to file err: %s", err)
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("err while receiving file %s", err)
		}
	}

	logrus.Println("Collecting results ended")
	logrus.Printf("File is located at %s", filePath)
}
