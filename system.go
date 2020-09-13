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
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	zmq "github.com/pebbe/zmq4"
)

// ProcessMessage is a function signature definition for remote message processing
type ProcessMessage func(msg string, to Coordinates, from Coordinates)

// System provides support for graceful shutdown
type System struct {
	Name      string
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan interface{}
	IsReady   chan interface{}
	CanStart  chan interface{}
	actors    *actorsMap
	onMessage ProcessMessage
	host      string
	publish   chan string
	receive   chan string
}

// New returns new actor system fascade
func New(parentCtx context.Context, name string, lakeHostname string) System {
	ctx, cancel := context.WithCancel(parentCtx)

	if lakeHostname == "" {
		panic(fmt.Errorf("invalid lake hostname").Error())
	}
	if name == "" || name == "[" {
		panic(fmt.Errorf("invalid system name").Error())
	}

	return System{
		Name:     name,
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan interface{}),
		IsReady:  make(chan interface{}),
		CanStart: make(chan interface{}),
		actors: &actorsMap{
			underlying: make(map[string]*Actor),
		},
		host:      lakeHostname,
		onMessage: func(msg string, to Coordinates, from Coordinates) {},
		publish:   make(chan string),
		receive:   make(chan string),
	}
}

func (s *System) workPush() {
	var (
		chunk   string
		channel *zmq.Socket
		err     error
	)

	runtime.LockOSThread()
	defer func() {
		recover()
		s.Stop()
		runtime.UnlockOSThread()
	}()

	ctx, err := zmq.NewContext()
	if err != nil {
		return
	}

	go func() {
		<-s.Done()
		ctx.Term()
	}()

pushCreation:
	channel, err = ctx.NewSocket(zmq.PUSH)
	if err != nil && err.Error() == "resource temporarily unavailable" {
		select {
		case <-time.After(100 * time.Millisecond):
			goto pushCreation
		}
	} else if err != nil {
		return
	}
	channel.SetConflate(false)
	channel.SetImmediate(true)
	channel.SetSndhwm(0)
	defer channel.Close()

pushConnection:
	err = channel.Connect(fmt.Sprintf("tcp://%s:%d", s.host, 5562))
	if err != nil && (err == zmq.ErrorSocketClosed || err == zmq.ErrorContextClosed || zmq.AsErrno(err) == zmq.ETERM) {
		return
	} else if err != nil {
		select {
		case <-time.After(100 * time.Millisecond):
			goto pushConnection
		}
	}

loop:
	select {
	case chunk = <-s.publish:
		if chunk == "" {
			goto loop
		}
		_, err = channel.Send(chunk, 0)
		if err != nil {
			goto eos
		}
	}
	goto loop

eos:
	s.Stop()
	return
}

func (s *System) workSub() {
	var (
		chunk   string
		channel *zmq.Socket
		err     error
	)

	runtime.LockOSThread()
	defer func() {
		recover()
		s.Stop()
		runtime.UnlockOSThread()
	}()

	ctx, err := zmq.NewContext()
	if err != nil {
		return
	}

	go func() {
		<-s.Done()
		ctx.Term()
	}()

subCreation:
	channel, err = ctx.NewSocket(zmq.SUB)
	if err != nil && err.Error() == "resource temporarily unavailable" {
		select {
		case <-time.After(100 * time.Millisecond):
			goto subCreation
		}
	} else if err != nil {
		return
	}
	channel.SetConflate(false)
	channel.SetImmediate(true)
	channel.SetRcvhwm(0)
	defer channel.Close()

subConnection:
	err = channel.Connect(fmt.Sprintf("tcp://%s:%d", s.host, 5561))
	if err != nil && (err == zmq.ErrorSocketClosed || err == zmq.ErrorContextClosed || zmq.AsErrno(err) == zmq.ETERM) {
		return
	} else if err != nil {
		select {
		case <-time.After(100 * time.Millisecond):
			goto subConnection
		}
	}

	if err = channel.SetSubscribe(s.Name + " "); err != nil {
		return
	}
	defer channel.SetUnsubscribe(s.Name + " ")

loop:
	chunk, err = channel.Recv(0)
	if err != nil && (err == zmq.ErrorSocketClosed || err == zmq.ErrorContextClosed || zmq.AsErrno(err) == zmq.ETERM) {
		goto eos
	}
	s.receive <- chunk
	goto loop

eos:
	s.Stop()
	return
}

// RegisterOnMessage register callback on message receive
func (s *System) RegisterOnMessage(cb ProcessMessage) {
	if s == nil {
		return
	}
	s.onMessage = cb
}

// RegisterActor register new actor into actor system
func (s *System) RegisterActor(ref *Actor, initialReaction func(interface{}, Context)) (err error) {
	if s == nil || ref == nil {
		return
	}
	_, exists := s.actors.Load(ref.Name)
	if exists {
		return
	}

	ref.React(initialReaction)
	s.actors.Store(ref.Name, ref)

	go func() {
		for {
			select {
			case <-s.Done():
				return
			case p := <-ref.Backlog:
				ref.Receive(p)
			case <-ref.Exit:
				return
			}
		}
	}()

	return
}

// ActorOf return actor reference by name
func (s *System) ActorOf(name string) (*Actor, error) {
	if s == nil {
		return nil, fmt.Errorf("cannot call method on nil reference")
	}
	ref, exists := s.actors.Load(name)
	if !exists {
		return nil, fmt.Errorf("actor %v not registered", name)
	}
	return ref, nil
}

// UnregisterActor stops actor and removes it from actor system
func (s *System) UnregisterActor(name string) {
	if s == nil {
		return
	}
	ref, err := s.ActorOf(name)
	if err != nil {
		return
	}
	s.actors.Delete(name)
	close(ref.Exit)
}

// SendMessage send message to to local of remote actor system
func (s *System) SendMessage(msg string, to Coordinates, from Coordinates) {
	if s == nil {
		return
	}
	if to.Region == from.Region {
		s.onMessage(msg, to, from)
	} else {
		s.publish <- (to.Region + " " + from.Region + " " + to.Name + " " + from.Name + " " + msg)
	}
}

// WaitReady wait for daemon to be ready within given deadline
func (s *System) WaitReady(deadline time.Duration) (err error) {
	if s.IsCanceled() {
		return
	}
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case string:
				err = fmt.Errorf(x)
			case error:
				err = x
			default:
				err = fmt.Errorf("actor-system %s unknown panic", s.Name)
			}
		}
	}()

	ticker := time.NewTicker(deadline)
	select {
	case <-s.IsReady:
		ticker.Stop()
		err = nil
		return
	case <-ticker.C:
		err = fmt.Errorf("actor-system %s was not ready within %v seconds", s.Name, deadline)
		return
	}
}

// WaitStop cancels context
func (s *System) WaitStop() {
	if s == nil {
		return
	}
	<-s.done
}

// GreenLight signals daemon to start work
func (s *System) GreenLight() {
	if s.IsCanceled() {
		return
	}
	s.CanStart <- nil
}

// MarkDone signals daemon is finished
func (s *System) MarkDone() {
	if s == nil {
		return
	}
	close(s.done)
}

// IsCanceled returns if daemon is done
func (s *System) IsCanceled() bool {
	if s == nil {
		return false
	}
	return s.ctx.Err() != nil
}

// MarkReady signals daemon is ready
func (s *System) MarkReady() {
	if s == nil {
		return
	}
	s.IsReady <- nil
}

// Done cancel channel
func (s *System) Done() <-chan struct{} {
	if s == nil {
		stub := make(chan struct{})
		close(stub)
		return stub
	}
	return s.ctx.Done()
}

// Stop cancels context
func (s *System) Stop() {
	if s == nil {
		return
	}
	s.cancel()
}

func (s *System) handshake() {
	if s == nil {
		return
	}
	var stash []string
	pingMessage := s.Name + " ]"

	ticker := time.NewTicker(400 * time.Millisecond)

	for {
		select {
		case <-s.Done():
			return
		case data := <-s.receive:
			if data != pingMessage {
				stash = append(stash, data)
				continue
			}
			ticker.Stop()
			for _, data := range stash {
				s.receive <- data
			}
			return
		case <-ticker.C:
			s.publish <- pingMessage
		}
	}
}

// Start handles everything needed to start actor-system
func (s *System) Start() {
	if s == nil {
		return
	}
	go s.workPush()
	go s.workSub()

	s.handshake()
	s.MarkReady()

	select {
	case <-s.CanStart:
		break
	case <-s.Done():
		s.Stop()
		s.MarkDone()
		return
	}

	go func() {
		for {
			select {
			case message := <-s.receive:
				parts := strings.SplitN(message, " ", 5)
				if len(parts) < 4 {
					continue
				}
				s.onMessage(
					parts[4],
					Coordinates{
						Name:   parts[2],
						Region: parts[0],
					},
					Coordinates{
						Name:   parts[3],
						Region: parts[1],
					},
				)
			case <-s.Done():
				for actorName := range s.actors.underlying {
					s.UnregisterActor(actorName)
				}
				s.MarkDone()
				return
			}
		}
	}()

	s.WaitStop()
}
