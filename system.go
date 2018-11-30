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

package actor_system

import (
	"context"
	"fmt"

	lake "github.com/jancajthaml-openbank/lake-client/go"
	log "github.com/sirupsen/logrus"
)

// ActorSystemSupport represents actor system capabilities
type ActorSystemSupport struct {
	ctx        context.Context
	cancel     context.CancelFunc
	exitSignal chan struct{}
	IsReady    chan interface{}
	Actors     *ActorsMap
	lakeClient *lake.Client
	Name       string
}

// NewActorSystemSupport constructor
func NewActorSystemSupport(parentCtx context.Context, systemName string, lakeHostname string) ActorSystemSupport {
	ctx, cancel := context.WithCancel(parentCtx)
	lakeClient, err := lake.NewClient(ctx, systemName, lakeHostname)
	if err != nil {
		panic(err.Error())
	}

	return ActorSystemSupport{
		ctx:        ctx,
		cancel:     cancel,
		exitSignal: make(chan struct{}),
		IsReady:    make(chan interface{}),
		Actors: &ActorsMap{
			underlying: make(map[string]*Envelope),
		},
		lakeClient: lakeClient,
		Name:       systemName,
	}
}

// RegisterActor register new actor into actor system
func (s ActorSystemSupport) RegisterActor(ref *Envelope, initialState ActorRecieveFn) (err error) {
	if ref == nil {
		return
	}
	_, exists := s.Actors.Load(ref.Name)
	if exists {
		return
	}

	ref.React(initialState)
	s.Actors.Store(ref.Name, ref)

	go func() {
		defer func() {
			if e := recover(); e != nil {
				switch x := e.(type) {
				case string:
					err = fmt.Errorf(x)
				case error:
					err = x
				default:
					err = fmt.Errorf("Unknown panic")
				}
			}
		}()

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
func (s ActorSystemSupport) ActorOf(name string) (*Envelope, error) {
	ref, exists := s.Actors.Load(name)
	if !exists {
		return nil, fmt.Errorf("actor %v not registered", name)
	}

	return ref, nil
}

// UnregisterActor stops actor and removes it from actor system
func (s ActorSystemSupport) UnregisterActor(name string) {
	ref, err := s.ActorOf(name)
	if err != nil {
		return
	}

	s.Actors.Delete(name)
	ref.Exit <- nil
	close(ref.Backlog)
	close(ref.Exit)
}

// SendRemote send message to remote region
func (s ActorSystemSupport) SendRemote(destinationSystem, data string) {
	s.lakeClient.Publish <- []string{destinationSystem, data}
}

// ProcessLocalMessage send local message to actor by name
func (s ActorSystemSupport) ProcessLocalMessage(msg interface{}, receiver string, sender Coordinates) {
	log.Debugf("Actor System %+v recieved local message %+v", s.Name, msg)
}

func (s ActorSystemSupport) ProcessRemoteMessage(parts []string) {
	log.Debugf("Actor System %s recieved remote message %+v", s.Name, parts)
	if len(parts) != 4 {
		log.Warnf("Invalid message received [%+v remote]", parts)
		return
	}

	region, receiver, sender, payload := parts[0], parts[1], parts[2], parts[3]

	s.ProcessLocalMessage(payload, receiver, Coordinates{
		Name:   sender,
		Region: region,
	})
}

// Stop actor system and flush all actors
func (s ActorSystemSupport) Stop() {
	for actorName := range s.Actors.underlying {
		s.UnregisterActor(actorName)
	}

	s.cancel()
	<-s.exitSignal
}

// MarkDone signals actor system is finished
func (s ActorSystemSupport) MarkDone() {
	close(s.exitSignal)
}

// Done cancel channel
func (s ActorSystemSupport) Done() <-chan struct{} {
	return s.ctx.Done()
}

// MarkReady signals daemon is ready
func (s ActorSystemSupport) MarkReady() {
	s.IsReady <- nil
}

// Start handles everything needed to start metrics daemon
func (s ActorSystemSupport) Start() {
	defer s.MarkDone()

	log.Info("Starting Actor System")
	s.lakeClient.Start()
	s.MarkReady()
	log.Info("Start Actor System")

	for {
		select {
		case message := <-s.lakeClient.Receive:
			s.ProcessRemoteMessage(message)
		case <-s.Done():
			log.Info("Stopping Actor System")
			s.lakeClient.Stop()
			log.Info("Stop Actor System")
			return
		}
	}
}
