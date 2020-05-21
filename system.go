// Copyright (c) 2016-2019, Jan Cajthaml <jan.cajthaml@gmail.com>
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
	log "github.com/sirupsen/logrus"
)

const backoff = 500 * time.Millisecond

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
	push      chan string
	sub       chan string
	Publish   chan<- string
	Receive   <-chan string
}

// NewSystem constructor
func NewSystem(parentCtx context.Context, name string, lakeHostname string) System {
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
			underlying: make(map[string]*Envelope),
		},
		push: make(chan string),
		sub:  make(chan string),
		host: lakeHostname,
		onMessage: func(msg string, to Coordinates, from Coordinates) {
			log.Warnf("[Call OnMessage] actor-system-%s received message %+v from: %+v to: %+v", name, msg, from, to)
		},
	}
}

func (s *System) workZMQSub(ctx context.Context, cancel context.CancelFunc) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer cancel()
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case string:
				log.Warnf("SUB recovered from crash %s", x)
			case error:
				log.Warnf("SUB recovered from crash %+v", x)
			default:
				log.Warn("SUB recovered from crash")
			}
		}
	}()

	var (
		chunk   string
		channel *zmq.Socket
		err     error
	)

subCreation:
	channel, err = zmq.NewSocket(zmq.SUB)
	if err != nil && err.Error() == "resource temporarily unavailable" {
		log.Warn("Resources unavailable in connect")
		select {
		case <-time.After(backoff):
			goto subCreation
		}
	} else if err != nil {
		log.Warn("Unable to connect SUB socket ", err)
		return
	}
	channel.SetConflate(false)
	channel.SetImmediate(true)
	channel.SetRcvhwm(0)
	defer channel.Close()

subConnection:
	err = channel.Connect(fmt.Sprintf("tcp://%s:%d", s.host, 5561))
	if err != nil {
		log.Warn("Unable to connect to SUB address ", err)
		select {
		case <-time.After(backoff):
			goto subConnection
		}
	}

	if err = channel.SetSubscribe(s.Name + " "); err != nil {
		log.Warnf("Subscription to %s failed with: %+v", s.Name, err)
		return
	}
	defer channel.SetUnsubscribe(s.Name + " ")

loop:
	if ctx.Err() != nil {
		return
	}

	chunk, err = channel.Recv(0)
	switch err {
	case nil:
		s.sub <- chunk
	case zmq.ErrorSocketClosed, zmq.ErrorContextClosed:
		log.Warnf("ZMQ connection closed %+v", err)
		return
	default:
		log.Warnf("Error while receiving ZMQ message %+v", err)
	}
	goto loop
}

func (s *System) workZMQPush(ctx context.Context, cancel context.CancelFunc) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer cancel()
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case string:
				log.Warnf("ZMQ push recovered from crash %s", x)
			case error:
				log.Warnf("ZMQ push recovered from crash %+v", x)
			default:
				log.Warn("ZMQ push recovered from crash")
			}
		}
	}()

	var (
		chunk   string
		channel *zmq.Socket
		err     error
	)

pushCreation:
	channel, err = zmq.NewSocket(zmq.PUSH)
	if err != nil && err.Error() == "resource temporarily unavailable" {
		log.Warn("Resources unavailable in connect")
		select {
		case <-time.After(backoff):
			goto pushCreation
		}
	} else if err != nil {
		log.Warn("Unable to connect ZMQ socket ", err)
		return
	}
	channel.SetConflate(false)
	channel.SetImmediate(true)
	channel.SetSndhwm(0)
	defer channel.Close()

pushConnection:
	err = channel.Connect(fmt.Sprintf("tcp://%s:%d", s.host, 5562))
	if err != nil {
		log.Warn("Unable to connect to ZMQ address ", err)
		select {
		case <-time.After(backoff):
			goto pushConnection
		}
	}

loop:
	chunk = <-s.push
	if ctx.Err() == nil {
		channel.Send(chunk, 0)
		goto loop
	}
}

func (s *System) createPushChannel() chan<- string {
	in := make(chan string)

	go func() {
	loop:
		ctx, cancel := context.WithCancel(s.ctx)
		go s.workZMQPush(ctx, cancel)
		<-ctx.Done()
		if ctx.Err() == nil {
			goto loop
		}
	}()

	var data string

	go func() {
	push:
		select {
		case data = <-in:
			if data == "" {
				goto push
			}
			s.push <- data
		case <-s.Done():
			goto sink
		}
		goto push
	sink:
		data = <-in
		s.sub <- ""
		goto sink
	}()

	return in
}

func (s *System) createRecieveChannel() <-chan string {
	out := make(chan string)

	go func() {
	loop:
		ctx, cancel := context.WithCancel(s.ctx)
		go s.workZMQSub(ctx, cancel)
		<-ctx.Done()
		if ctx.Err() == nil {
			goto loop
		}
	}()

	pingMessage := s.Name + " ]"

	var stash []string
	var data string

	go func() {
	handshake:
		s.push <- pingMessage
		select {
		case data = <-s.sub:
			if data != pingMessage {
				stash = append(stash, data)
				goto handshake
			}
			for _, data := range stash {
				s.sub <- data
			}
			goto pull
		case <-s.Done():
			goto sink
		case <-time.After(backoff):
			goto handshake
		}

	pull:
		select {
		case data = <-s.sub:
			if data == pingMessage {
				goto pull
			}
			out <- data
		case <-s.Done():
			goto sink
		}
		goto pull
	sink:
		select {
		case <-s.sub:
			out <- ""
		case <-time.After(backoff):
			out <- ""
		}
		goto sink
	}()

	return out
}

// RegisterOnMessage register callback on message receive
func (s *System) RegisterOnMessage(cb ProcessMessage) {
	s.onMessage = cb
}

// RegisterActor register new actor into actor system
func (s *System) RegisterActor(ref *Envelope, initialReaction func(interface{}, Context)) (err error) {
	if ref == nil {
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
func (s *System) ActorOf(name string) (*Envelope, error) {
	ref, exists := s.actors.Load(name)
	if !exists {
		return nil, fmt.Errorf("actor %v not registered", name)
	}
	return ref, nil
}

// UnregisterActor stops actor and removes it from actor system
func (s *System) UnregisterActor(name string) {
	ref, err := s.ActorOf(name)
	if err != nil {
		return
	}
	s.actors.Delete(name)
	ref.Exit <- nil
	close(ref.Backlog)
	close(ref.Exit)
}

// SendMessage send message to to local of remote actor system
func (s *System) SendMessage(msg string, to Coordinates, from Coordinates) {
	if to.Region == from.Region {
		s.onMessage(msg, to, from)
	} else {
		s.Publish <- to.Region + " " + from.Region + " " + to.Name + " " + from.Name + " " + msg
	}
}

// WaitReady wait for daemon to be ready within given deadline
func (s *System) WaitReady(deadline time.Duration) (err error) {
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case string:
				err = fmt.Errorf(x)
			case error:
				err = x
			default:
				err = fmt.Errorf("actor-system-%s unknown panic", s.Name)
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
		err = fmt.Errorf("actor-system-%s was not ready within %v seconds", s.Name, deadline)
		return
	}
}

// WaitStop cancels context
func (s *System) WaitStop() {
	<-s.done
}

// GreenLight signals daemon to start work
func (s *System) GreenLight() {
	s.CanStart <- nil
}

// MarkDone signals daemon is finished
func (s *System) MarkDone() {
	close(s.done)
}

// IsCanceled returns if daemon is done
func (s *System) IsCanceled() bool {
	return s.ctx.Err() != nil
}

// MarkReady signals daemon is ready
func (s *System) MarkReady() {
	s.IsReady <- nil
}

// Done cancel channel
func (s *System) Done() <-chan struct{} {
	return s.ctx.Done()
}

// Stop cancels context
func (s *System) Stop() {
	s.cancel()
}

// Start handles everything needed to start actor-system
func (s *System) Start() {
	s.MarkReady()

	select {
	case <-s.CanStart:
		s.Publish = s.createPushChannel()
		s.Receive = s.createRecieveChannel()
		break
	case <-s.Done():
		s.MarkDone()
		return
	}

	log.Infof("Start actor-system-%s", s.Name)

	go func() {
		for {
			select {
			case message := <-s.Receive:
				parts := strings.SplitN(message, " ", 5)

				if len(parts) < 4 {
					log.Warnf("Invalid message received [%+v]", parts)
					continue
				}
				recieverRegion, senderRegion, receiverName, senderName := parts[0], parts[1], parts[2], parts[3]
				from := Coordinates{
					Name:   senderName,
					Region: senderRegion,
				}
				to := Coordinates{
					Name:   receiverName,
					Region: recieverRegion,
				}
				s.onMessage(parts[4], to, from)
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
	log.Infof("Stop actor-system-%s", s.Name)
}
