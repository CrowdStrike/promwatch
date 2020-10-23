// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNaiveStore(t *testing.T) {
	s := NewStore()
	t1 := "This is a test"
	t2 := "More of everything!"

	assert.Equal(t, "", s.String(), "Store should be empty initially")

	s.Add(t1)
	expected := t1
	assert.Equal(t, "", s.String(), "Store should be empty before commit")
	s.Commit()
	assert.Equal(t, expected, s.String())

	s.Add(t1)
	s.Add(t2)
	assert.Equal(t, expected, s.String(), "Store should contain previous value before commit")
	expected = t1 + t2
	s.Commit()
	assert.Equal(t, expected, s.String(), "Store should contain both added values after commit")

	n := s.(*naiveStore)
	assert.Equal(t, "", n.internal.String(), "Internal buffer should be empty after commit")
}
