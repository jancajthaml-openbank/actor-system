// Copyright (c) 2016-2018, Jan Cajthaml <jan.cajthaml@gmail.com>
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

	lake "github.com/jancajthaml-openbank/lake-client/go"
	log "github.com/sirupsen/logrus"
)

// ProcessLocalMessage is a function signature definition for local message processing
type ProcessLocalMessage func(msg interface{}, to Coordinates, from Coordinates)

// ProcessRemoteMessage is a function signature definition for remote message processing
type ProcessRemoteMessage func(parts []string)

// Support represents actor system capabilities
type Support struct {
	Name            string
	IsReady         chan interface{}
	ctx             context.Context
	cancel          context.CancelFunc
	exitSignal      chan struct{}
	actors          *actorsMap
	lakeClient      *lake.Client
	onLocalMessage  ProcessLocalMessage
	onRemoteMessage ProcessRemoteMessage
}

// NewSupport constructor
func NewSupport(parentCtx context.Context, systemName string, lakeHostname string) Support {
	ctx, cancel := context.WithCancel(parentCtx)
	lakeClient, err := lake.NewClient(ctx, systemName, lakeHostname)
	if err != nil {
		panic(err.Error())
	}

	return Support{
		ctx:        ctx,
		cancel:     cancel,
		exitSignal: make(chan struct{}),
		IsReady:    make(chan interface{}),
		actors: &actorsMap{
			underlying: make(map[string]*Envelope),
		},
		lakeClient: lakeClient,
		Name:       systemName,
	}
}

// RegisterOnLocalMessage register callback on local message receive
func (s *Support) RegisterOnLocalMessage(cb ProcessLocalMessage) {
	s.onLocalMessage = cb
}

// RegisterOnRemoteMessage register callback on remote message receive
func (s *Support) RegisterOnRemoteMessage(cb ProcessRemoteMessage) {
	s.onRemoteMessage = cb
}

// RegisterActor register new actor into actor system
func (s *Support) RegisterActor(ref *Envelope, initialReaction func(interface{}, Context)) (err error) {
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
func (s *Support) ActorOf(name string) (*Envelope, error) {
	ref, exists := s.actors.Load(name)
	if !exists {
		return nil, fmt.Errorf("actor %v not registered", name)
	}

	return ref, nil
}

// UnregisterActor stops actor and removes it from actor system
func (s *Support) UnregisterActor(name string) {
	ref, err := s.ActorOf(name)
	if err != nil {
		return
	}

	s.actors.Delete(name)
	ref.Exit <- nil
	close(ref.Backlog)
	close(ref.Exit)
}

// SendRemote send message to remote region
func (s *Support) SendRemote(destinationSystem, data string) {
	s.lakeClient.Publish <- []string{destinationSystem, data}
}

// BroadcastRemote send message to all remote regions
func (s *Support) BroadcastRemote(data string) {
	s.lakeClient.Broadcast(data)
}

// Stop actor system and flush all actors
func (s *Support) Stop() {
	for actorName := range s.actors.underlying {
		s.UnregisterActor(actorName)
	}

	s.cancel()
	<-s.exitSignal
}

// MarkDone signals actor system is finished
func (s *Support) MarkDone() {
	close(s.exitSignal)
}

// Done cancel channel
func (s *Support) Done() <-chan struct{} {
	return s.ctx.Done()
}

// EnsureContract check if contract of embedding is ok and marks ready
func (s *Support) EnsureContract() {
	if s.onRemoteMessage == nil {
		s.onLocalMessage = func(msg interface{}, to Coordinates, from Coordinates) {
			log.Warnf("[Call RegisterOnLocalMessage] Actor System %+v received local message %+v", s.Name, msg)
		}
	}

	if s.onRemoteMessage == nil {
		s.onRemoteMessage = func(parts []string) {
			log.Warnf("[Call RegisterOnRemoteMessage] Actor System %s received remote message %+v", s.Name, parts)
		}
	}

	s.IsReady <- nil
}

// Start handles everything needed to start metrics daemon
func (s *Support) Start() {
	defer s.MarkDone()

	log.Info("Starting Actor System")
	s.lakeClient.Start()
	s.EnsureContract()
	log.Info("Start Actor System")

	for {
		select {
		case message := <-s.lakeClient.Receive:
			s.onRemoteMessage(message)
		case <-s.Done():
			log.Info("Stopping Actor System")
			s.lakeClient.Stop()
			log.Info("Stop Actor System")
			return
		}
	}
}
