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

package tail

import (
	"bytes"
	"github.com/chriskery/slurm-bridge-operator/pkg/tail"
	"io"
	"log"
	"sync"
)

type Reader struct {
	t        *tail.Tail
	isClosed bool

	mu   sync.Mutex
	buff *bytes.Buffer
}

func NewReader(path string) (*Reader, error) {
	t, err := tail.TailFile(path, tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		return nil, err
	}

	tr := &Reader{
		t:    t,
		buff: &bytes.Buffer{},
	}

	go tr.readTail()

	return tr, nil
}

// Read returns EOF error only after invoking Close.
// Before close in case of EOF errors it will be returning nil.
func (tr *Reader) Read(p []byte) (int, error) {
	tr.mu.Lock()
	n, err := io.ReadFull(tr.buff, p)
	tr.mu.Unlock()
	if (err == io.EOF || err == io.ErrUnexpectedEOF) && !tr.isClosed {
		return n, nil
	}

	return n, err
}

func (tr *Reader) Close() error {
	_ = tr.t.StopAtEOF() // it returns stop reason instead of err
	return nil
}

func (tr *Reader) readTail() {
	defer func() {
		log.Println("Read tail finished")
		tr.isClosed = true
	}()

	for {
		l, ok := <-tr.t.Lines
		if !ok {
			return
		}

		if l.Err != nil {
			log.Printf("Tail line err: %s", l.Err)
			return
		}

		tr.mu.Lock()
		_, err := tr.buff.WriteString(l.Text + "\n")
		tr.mu.Unlock()
		if err != nil {
			log.Printf("Could not write to buffer err: %s", err)
			return
		}
	}
}
