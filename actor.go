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
	"sync"
)

type actorsMap struct {
	sync.RWMutex
	underlying map[string]*Actor
}

// Load works same as get from map
func (rm *actorsMap) Load(key string) (value *Actor, ok bool) {
	rm.RLock()
	defer rm.RUnlock()
	result, ok := rm.underlying[key]
	return result, ok
}

// Delete works same as delete from map
func (rm *actorsMap) Delete(key string) {
	rm.Lock()
	defer rm.Unlock()
	delete(rm.underlying, key)
}

// Store works same as store to map
func (rm *actorsMap) Store(key string, value *Actor) {
	rm.Lock()
	defer rm.Unlock()
	rm.underlying[key] = value
}

// Coordinates represents actor namespace
type Coordinates struct {
	Name   string
	Region string
}

func (ref *Coordinates) String() string {
	return ref.Region + "/" + ref.Name
}

// Context represents actor message envelope
type Context struct {
	Data     interface{}
	Self     *Actor
	Receiver Coordinates
	Sender   Coordinates
}

// Actor represents single actor
type Actor struct {
	Name    string
	State   interface{}
	receive func(interface{}, Context)
	Backlog chan Context
	Exit    chan interface{}
}

// NewActor returns new actor instance
func NewActor(name string, state interface{}) *Actor {
	return &Actor{
		Name:    name,
		State:   state,
		Backlog: make(chan Context, 64),
		Exit:    make(chan interface{}),
	}
}

// Tell queues message to actor
func (ref *Actor) Tell(data interface{}, receiver Coordinates, sender Coordinates) (err error) {
	if ref == nil {
		err = fmt.Errorf("actor reference %v not found", ref)
		return
	}
	ref.Backlog <- Context{
		Data:     data,
		Self:     ref,
		Receiver: receiver,
		Sender:   sender,
	}
	return
}

// Become transforms actor behavior for next message
func (ref *Actor) Become(state interface{}, f func(interface{}, Context)) {
	if ref == nil {
		return
	}
	ref.State = state
	ref.React(f)
	return
}

func (ref *Actor) String() string {
	if ref == nil {
		return "<Deadletter>"
	}
	return ref.Name
}

// React change become function
func (ref *Actor) React(f func(interface{}, Context)) {
	if ref == nil {
		return
	}
	ref.receive = f
	return
}

// Receive dequeues message to actor
func (ref *Actor) Receive(msg Context) {
	if ref.receive == nil {
		return
	}
	ref.receive(ref.State, msg)
}
