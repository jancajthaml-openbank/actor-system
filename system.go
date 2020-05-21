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
	log "github.com/sirupsen/logrus"
)

const backoff = 100 * time.Millisecond

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
		host: lakeHostname,
		onMessage: func(msg string, to Coordinates, from Coordinates) {
			log.Warnf("[Call OnMessage] actor-system %s received message %+v from: %+v to: %+v", name, msg, from, to)
		},
		publish: make(chan string),
		receive: make(chan string),
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
		log.Warnf("Unable to create ZMQ context %+v", err)
		return
	}

	go func() {
		<-s.Done()
		ctx.Term()
	}()

pushCreation:
	channel, err = ctx.NewSocket(zmq.PUSH)
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
	if err != nil && (err == zmq.ErrorSocketClosed || err == zmq.ErrorContextClosed || zmq.AsErrno(err) == zmq.ETERM) {
		return
	} else if err != nil {
		log.Warn("Unable to connect to ZMQ address ", err)
		select {
		case <-time.After(backoff):
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
			log.Warnf("Unable to send message error: %+v", err)
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
		log.Warnf("Unable to create ZMQ context %+v", err)
		return
	}

	go func() {
		<-s.Done()
		ctx.Term()
	}()

subCreation:
	channel, err = ctx.NewSocket(zmq.SUB)
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
	if err != nil && (err == zmq.ErrorSocketClosed || err == zmq.ErrorContextClosed || zmq.AsErrno(err) == zmq.ETERM) {
		return
	} else if err != nil {
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
	chunk, err = channel.Recv(0)
	if err != nil && (err == zmq.ErrorSocketClosed || err == zmq.ErrorContextClosed || zmq.AsErrno(err) == zmq.ETERM) {
		log.Warnf("SUB stopping with %+v", err)
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
				log.Infof("Actor %s Stopping", ref.Name)
				return
			case p := <-ref.Backlog:
				ref.Receive(p)
			case <-ref.Exit:
				log.Infof("Actor %s Stopping", ref.Name)
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
	close(ref.Exit)
}

// SendMessage send message to to local of remote actor system
func (s *System) SendMessage(msg string, to Coordinates, from Coordinates) {
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

func (s *System) handshake() {

	var stash []string
	pingMessage := s.Name + " ]"

	ticker := time.NewTicker(500 * time.Millisecond)

	for {
		log.Infof("Start actor-system %s performing handshake", s.Name)
		s.publish <- pingMessage
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
			continue
		}
	}
}

// Start handles everything needed to start actor-system
func (s *System) Start() {
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

	log.Infof("Start actor-system %s", s.Name)

	go func() {
		for {
			select {
			case message := <-s.receive:
				log.Infof("Received message [%+v]", message)

				parts := strings.SplitN(message, " ", 5)
				if len(parts) < 4 {
					log.Warnf("Invalid message received [%+v]", parts)
					continue
				}
				recieverRegion, senderRegion, receiverName, senderName := parts[0], parts[1], parts[2], parts[3]
				s.onMessage(
					parts[4],
					Coordinates{
						Name:   receiverName,
						Region: recieverRegion,
					},
					Coordinates{
						Name:   senderName,
						Region: senderRegion,
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
	log.Infof("Stop actor-system %s", s.Name)
}
