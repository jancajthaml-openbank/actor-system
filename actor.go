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
	underlying map[string]*Envelope
}

// Load works same as get from map
func (rm *actorsMap) Load(key string) (value *Envelope, ok bool) {
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
func (rm *actorsMap) Store(key string, value *Envelope) {
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
	Self     *Envelope
	Receiver Coordinates
	Sender   Coordinates
}

// Envelope represents single actor
type Envelope struct {
	Name    string
	State   interface{}
	receive func(interface{}, Context)
	Backlog chan Context
	Exit    chan interface{}
}

// NewEnvelope returns new actor instance
func NewEnvelope(name string, state interface{}) *Envelope {
	return &Envelope{
		Name:    name,
		State:   state,
		Backlog: make(chan Context, 64),
		Exit:    make(chan interface{}),
	}
}

// Tell queues message to actor
func (ref *Envelope) Tell(data interface{}, receiver Coordinates, sender Coordinates) (err error) {
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

// Become transforms actor behaviour for next message
func (ref *Envelope) Become(state interface{}, f func(interface{}, Context)) {
	if ref == nil {
		return
	}
	ref.State = state
	ref.React(f)
	return
}

func (ref *Envelope) String() string {
	if ref == nil {
		return "<Deadletter>"
	}
	return ref.Name
}

// React change become function
func (ref *Envelope) React(f func(interface{}, Context)) {
	if ref == nil {
		return
	}
	ref.receive = f
	return
}

// Receive dequeues message to actor
func (ref *Envelope) Receive(msg Context) {
	if ref.receive == nil {
		return
	}
	ref.receive(ref.State, msg)
}
