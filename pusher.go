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
  "runtime"

  "github.com/pebbe/zmq4"
)

type Pusher struct {
  host   string
  ctx    *zmq4.Context
  Data   chan string
  socket *zmq4.Socket
}

func NewPusher(host string) Pusher {
  return Pusher{
    host: host,
    Data: make(chan string),
  }
}

func (s *Pusher) Stop() {
  if s == nil {
    return
  }
  if s.socket != nil {
    s.socket.Close()
  }
  if s.ctx != nil {
    for s.ctx.Term() != nil {}
  }
  s.socket = nil
  s.ctx = nil
}

func (s *Pusher) Start() error {
  var chunk   string
  var err     error

  runtime.LockOSThread()
  defer func() {
    recover()
    runtime.UnlockOSThread()
  }()

  s.ctx, err = zmq4.NewContext()
  if err != nil {
    return err
  }

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
    if chunk == "" {
      goto loop
    }
    _, err = s.socket.Send(chunk, 0)
    if err != nil {
      goto eos
    }
  }
  goto loop

eos:
  return nil
}
