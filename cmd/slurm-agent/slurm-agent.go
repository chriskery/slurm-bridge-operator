package main

import (
	"flag"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-agent"
	sgrpc "github.com/chriskery/slurm-bridge-operator/pkg/slurm-agent/api"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/sirupsen/logrus"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"
)

func init() {
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:03:04",
	})
}

func main() {
	configPath := flag.String("config", "", "path to a slurm-agent-agent config")
	sock := flag.String("socket", "/var/run/slurm-bridge-operator/slurm-agent-agent.sock", "unix socket to serve slurm-agent API")
	enableHttp := flag.Bool("enableHttp", true, "http to serve slurm-agent API")
	flag.Parse()

	config, err := config(*configPath)
	if err != nil {
		logrus.Fatal(err)
	}
	spew.Dump(config)

	if err = prepareSocketDir(sock); err != nil {
		logrus.Fatal(err)
	}

	ln, err := net.Listen("unix", *sock)
	if err != nil {
		logrus.Fatalf("Could not listen unix: %v", err)
	}

	s := grpc.NewServer()
	a := newSlurmClient(config)
	workload.RegisterWorkloadManagerServer(s, a)

	var wg sync.WaitGroup
	wg.Add(1)
	go runSignalHandler(&wg, s)

	if *enableHttp {
		go newHttpServer(s)
	}
	logrus.Infof("Starting server on %s", ln.Addr())
	if err = s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logrus.Fatalf("Could not serve requests: %v", err)
	}
	wg.Wait()
}

func runSignalHandler(wg *sync.WaitGroup, s *grpc.Server) {
	func() {
		defer wg.Done()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, unix.SIGINT, unix.SIGTERM, unix.SIGQUIT)
		logrus.Warnf("Shutting down due to %v", <-sig)
		s.GracefulStop()
	}()
}

func newSlurmClient(config sgrpc.Config) *sgrpc.Slurm {
	c, err := slurm_agent.NewClient()
	if err != nil {
		logrus.Fatalf("Could not create slurm-agent client: %s", err)
	}
	a := sgrpc.NewSlurm(c, config)
	return a
}

func prepareSocketDir(sock *string) error {
	if !fileutil.Exist(filepath.Dir(*sock)) {
		if err := os.MkdirAll(filepath.Dir(*sock), os.ModePerm); err != nil {
			return err
		}
	}
	if fileutil.Exist(*sock) {
		return os.RemoveAll(*sock)
	}
	return nil
}

func newHttpServer(s *grpc.Server) {
	ln, err := net.Listen("tcp", ":9999")
	if err != nil {
		logrus.Fatalf("Could not listen http: %v", err)
	}
	logrus.Infof("Starting server on %s", ln.Addr())
	if err = s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logrus.Fatalf("Could not serve requests: %v", err)
	}
}

func config(path string) (sgrpc.Config, error) {
	if path == "" {
		// sgrpc.Config is a map under the hood, and the default config is empty.
		// Here we return default value for map - nil. This will make any further
		// read successful, and fetched values will be empty PartitionResources.
		return sgrpc.Config(nil), nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "could not open config file")
	}
	defer file.Close()

	var c sgrpc.Config
	err = yaml.NewDecoder(file).Decode(&c)
	return c, errors.Wrapf(err, "could not decode config")
}
