// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"bytes"
	"sync"
)

// Store provides methods to store and retrieve strings.
type Store interface {
	Add(str string)
	Commit()
	String() string
}

func NewStore() Store {
	return &naiveStore{
		internal: &bytes.Buffer{},
		view:     &bytes.Buffer{},
	}
}

type naiveStore struct {
	sync.Mutex

	internal *bytes.Buffer
	view     *bytes.Buffer
}

// Add appends a string to the store.
func (s *naiveStore) Add(str string) {
	s.Lock()
	defer s.Unlock()
	s.internal.WriteString(str)
}

// String returns the store as string.
func (s *naiveStore) String() string {
	s.Lock()
	defer s.Unlock()
	return s.view.String()
}

// Commit swaps the internal and external view buffers. This swap makes sure the
// external view contains the full set of metrics whenever requested.
func (s *naiveStore) Commit() {
	s.Lock()
	defer s.Unlock()
	s.internal, s.view = s.view, s.internal
	s.internal.Reset()
}
