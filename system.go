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
)

// ProcessMessage is a function signature definition for remote message processing
type ProcessMessage func(msg string, to Coordinates, from Coordinates)

// System provides support for graceful shutdown
type System struct {
	Name      string
	actors    *actorsMap
	onMessage ProcessMessage
	push      Pusher
	sub       Subber
	ctx       context.Context
	cancel    context.CancelFunc
}

// New returns new actor system fascade
func New(name string, lakeHostname string) (System, error) {
	if lakeHostname == "" {
		return System{}, fmt.Errorf("invalid lake hostname")
	}
	if name == "" {
		return System{}, fmt.Errorf("invalid system name")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return System{
		Name:   name,
		ctx:    ctx,
		cancel: cancel,
		actors: &actorsMap{
			underlying: make(map[string]*Actor),
		},
		onMessage: func(msg string, to Coordinates, from Coordinates) {},
		push:      NewPusher(lakeHostname),
		sub:       NewSubber(lakeHostname, name),
	}, nil
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
	ref := s.actors.Delete(name)
	if ref == nil {
		return
	}
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
		s.push.Data <- (to.Region + " " + from.Region + " " + to.Name + " " + from.Name + " " + msg)
	}
}

func (s *System) exhaustMailbox() {
	if s == nil {
		return
	}
	var message string
	var start int
	var end int
	var parts = make([]string, 5)
	var idx int
	var i int

loop:
	select {
	case <-s.ctx.Done():
		goto eos
	case message = <-s.sub.Data:
		start = 0
		end = len(message)
		idx = 0
		i = 0

		for i < end && idx < 4 {
			if message[i] == 32 {
				if !(start == i && message[start] == 32) {
					parts[idx] = message[start:i]
					idx++
				}
				start = i + 1
			}
			i++
		}
		if idx < 5 && message[start] != 32 && len(message[start:]) > 0 {
			parts[idx] = message[start:]
			idx++
		}
		if idx != 5 {
			goto loop
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
	}
	goto loop
eos:
	return
}

// Stop terminates work
func (s *System) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	<-s.ctx.Done()
}

// Start spins PUSH and SUB workers
func (s *System) Start() {
	if s == nil {
		return
	}

	go func() {
		s.sub.Start()
		s.cancel()
	}()
	defer s.sub.Stop()

	go func() {
		s.push.Start()
		s.cancel()
	}()
	defer s.push.Stop()

	defer func() {
		for actorName := range s.actors.underlying {
			s.UnregisterActor(actorName)
		}
	}()

	s.exhaustMailbox()
}
