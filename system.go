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
	"strings"
)

// ProcessMessage is a function signature definition for remote message processing
type ProcessMessage func(msg string, to Coordinates, from Coordinates)

// System provides support for graceful shutdown
type System struct {
	Name      string
	actors    *actorsMap
	onMessage ProcessMessage
	host      string
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
		host:      lakeHostname,
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
		s.push.Data <- (to.Region + " " + from.Region + " " + to.Name + " " + from.Name + " " + msg)
	}
}

func (s *System) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	<-s.ctx.Done()
}

// Start handles everything needed to start actor-system
func (s *System) Start() {
	if s == nil {
		return
	}

	go func() {
		s.push.Start()
		s.cancel()
	}()
	defer s.push.Stop()

	go func() {
		s.sub.Start()
		s.cancel()
	}()
	defer s.sub.Stop()

	defer func() {
		for actorName := range s.actors.underlying {
			s.UnregisterActor(actorName)
		}
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		case message := <-s.sub.Data:
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
		}
	}
}
