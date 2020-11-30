// Copyright (c) 2016-2020, Jan Cajthaml <jan.cajthaml@gmail.com>
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

package actorsystem

import (
	"fmt"
	"github.com/pebbe/zmq4"
	"runtime"
)

// Subber holds SUB socket wrapper
type Subber struct {
	host        string
	topic       string
	ctx         *zmq4.Context
	Data        chan string
	socket      *zmq4.Socket
	killedOrder chan interface{}
	deadConfirm chan interface{}
}

// NewSubber returns new SUB worker connected to host
func NewSubber(host string, topic string) Subber {
	return Subber{
		host:        host,
		topic:       topic,
		Data:        make(chan string),
		killedOrder: make(chan interface{}),
		deadConfirm: nil,
	}
}

// Stop closes socket and waits for zmq to terminate
func (s *Subber) Stop() {
	if s == nil {
		return
	}
	if s.deadConfirm != nil {
		close(s.killedOrder)
		<-s.deadConfirm
	}
	if s.socket != nil {
		s.socket.Close()
	}
	s.socket = nil
	s.ctx = nil
}

// Start creates SUB socket and relays all data from it to Data channel
func (s *Subber) Start() error {
	if s == nil {
		return fmt.Errorf("nil pointer")
	}

	var chunk string
	var err error

	runtime.LockOSThread()
	defer func() {
		recover()
		runtime.UnlockOSThread()
	}()

	s.ctx, err = zmq4.NewContext()
	if err != nil {
		return err
	}
	s.ctx.SetRetryAfterEINTR(false)

	for {
		s.socket, err = s.ctx.NewSocket(zmq4.SUB)
		if err == nil {
			break
		} else if err.Error() != "resource temporarily unavailable" {
			return err
		}
	}

	s.socket.SetConflate(false)
	s.socket.SetImmediate(true)
	s.socket.SetRcvhwm(0)
	s.socket.SetLinger(0)

	s.deadConfirm = make(chan interface{})
	defer close(s.deadConfirm)

	for {
		err = s.socket.Connect(fmt.Sprintf("tcp://%s:%d", s.host, 5561))
		if err == nil {
			break
		} else if err == zmq4.ErrorSocketClosed || err == zmq4.ErrorContextClosed || err == zmq4.ErrorNoSocket {
			return err
		}
	}

	if err = s.socket.SetSubscribe(s.topic + " "); err != nil {
		return err
	}
	defer s.socket.SetUnsubscribe(s.topic + " ")

loop:
	select {

	case <-s.killedOrder:
		goto eos
	default:
		chunk, err = s.socket.Recv(0)
		if err != nil && (err == zmq4.ErrorSocketClosed || err == zmq4.ErrorContextClosed || err == zmq4.ErrorNoSocket) {
			goto eos
		}
		s.Data <- chunk
		goto loop
	}

eos:
	return nil
}
