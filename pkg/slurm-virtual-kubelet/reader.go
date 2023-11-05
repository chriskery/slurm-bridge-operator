package slurm_virtual_kubelet

import (
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/pkg/errors"
	"io"
)

type OpenFileReader struct {
	OpenFileClient []workload.WorkloadManager_OpenFileClient
	readIndex      int
	buff           []byte
}

func (o *OpenFileReader) Read(p []byte) (n int, err error) {
	if o.readIndex >= len(o.OpenFileClient) {
		return 0, errors.New("Read index exceeds OpenFileClient length")
	}
readLoop:
	if len(o.buff) == 0 {
		chunk, err := o.OpenFileClient[o.readIndex].Recv()
		if err != nil {
			if o.readIndex == len(o.OpenFileClient)-1 {
				return 0, err
			}

			if !errors.Is(err, io.EOF) {
				return 0, err
			}

			o.readIndex++
			goto readLoop
		}
		o.buff = chunk.Content
	}

	readLen := len(p)
	if readLen > len(o.buff) {
		readLen = len(o.buff)
	}

	copy(p, o.buff[0:readLen])
	o.buff = o.buff[readLen:]
	return readLen, nil
}

type TailFileReader struct {
	workload.WorkloadManager_OpenFileClient
	buff []byte
}

func (o *TailFileReader) Read(p []byte) (n int, err error) {
	if len(o.buff) == 0 {
		chunk, err := o.Recv()
		if err != nil {
			return 0, err
		}
		o.buff = chunk.Content
	}

	readLen := len(p)
	if readLen > len(o.buff) {
		readLen = len(o.buff)
	}

	copy(p, o.buff[0:readLen])
	o.buff = o.buff[readLen:]
	return readLen, nil
}
