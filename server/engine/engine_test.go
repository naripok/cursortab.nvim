package engine

import (
	"cursortab/assert"
	"testing"
)

func TestEngineCreation(t *testing.T) {
	buf := newMockBuffer()
	prov := newMockProvider()
	clock := newMockClock()

	eng := createTestEngine(buf, prov, clock)

	assert.NotNil(t, eng, "NewEngine")
	assert.Equal(t, stateIdle, eng.state, "initial state")
}
