// Copyright (c) 2016-2021, Jan Cajthaml <jan.cajthaml@gmail.com>
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
	"time"
)

// Pusher holds PUSH socket wrapper
type Pusher struct {
	host        string
	ctx         *zmq4.Context
	Data        chan string
	socket      *zmq4.Socket
	killedOrder chan interface{}
	deadConfirm chan interface{}
}

// NewPusher returns new PUSH worker connected to host
func NewPusher(host string) Pusher {
	return Pusher{
		host:        host,
		Data:        make(chan string),
		killedOrder: make(chan interface{}),
		deadConfirm: nil,
	}
}

// Stop closes socket and waits for zmq to terminate
func (s *Pusher) Stop() {
	if s == nil {
		return
	}
	if s.deadConfirm != nil {
		close(s.killedOrder)
		select {
		case <-time.After(time.Second):
			break
		case <-s.deadConfirm:
			break
		}
	}
	if s.socket != nil {
		s.socket.Close()
		s.socket.Disconnect(fmt.Sprintf("tcp://%s:%d", s.host, 5562))
	}
	s.socket = nil
	s.ctx = nil
}

// Start creates PUSH socket and relays all Data chan to that socket
func (s *Pusher) Start() error {
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
		s.socket, err = s.ctx.NewSocket(zmq4.PUSH)
		if err == nil {
			break
		} else if err.Error() != "resource temporarily unavailable" {
			return err
		}
	}

	s.socket.SetConflate(false)
	s.socket.SetImmediate(true)
	s.socket.SetSndhwm(0)
	s.socket.SetLinger(0)

	s.deadConfirm = make(chan interface{})
	defer close(s.deadConfirm)

	for {
		err = s.socket.Connect(fmt.Sprintf("tcp://%s:%d", s.host, 5562))
		if err == nil {
			break
		} else if err == zmq4.ErrorSocketClosed || err == zmq4.ErrorContextClosed || err == zmq4.ErrorNoSocket {
			return err
		}
	}

loop:
	select {
	case chunk = <-s.Data:
	send:
		_, err = s.socket.SendBytes(StringToBytes(chunk), 0)
		if err != nil {
			if err.Error() != "resource temporarily unavailable" {
				time.Sleep(10 * time.Millisecond)
				goto send
			}
			goto eos
		}
	case <-s.killedOrder:
		goto eos
	}
	goto loop

eos:
	return nil
}
