package alert

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWait(t *testing.T) {
	wait := Notice{}
	var wg sync.WaitGroup
	errCh := make(chan string, 100)

	// standard alert
	wg.Add(100)
	for range 100 {
		go func() {
			w := wait.Wait(nil)
			wg.Done()
			if <-w {
				errCh <- "Wait should return false for alert notification"
			}
			wg.Done()
		}()
	}

	wg.Wait()
	wg.Add(100)
	isLeaky(t, &wait, nil)
	wait.Alert()
	wg.Wait()
	isLeaky(t, &wait, nil)
	assert.Empty(t, errCh, "Wait should not signal true when using Alert")

	// use kick
	ch := make(chan struct{})
	errCh = make(chan string, 100)
	wg.Add(100)
	for range 100 {
		go func() {
			w := wait.Wait(ch)
			wg.Done()
			if !<-w {
				errCh <- "Wait should return true when kick channel closes"
			}
			wg.Done()
		}()
	}
	wg.Wait()
	wg.Add(100)
	isLeaky(t, &wait, ch)
	close(ch)
	wg.Wait()
	ch = make(chan struct{})
	isLeaky(t, &wait, ch)
	assert.Empty(t, errCh, "Wait should signal true when kicked")

	// late receivers
	wg.Add(100)
	errCh = make(chan string, 100)
	for x := range 100 {
		go func(x int) {
			bb := wait.Wait(ch)
			wg.Done()
			if x%2 == 0 {
				time.Sleep(time.Millisecond * 5)
			}
			b := <-bb
			if b {
				errCh <- "Wait should return false for late receivers"
			}
			wg.Done()
		}(x)
	}
	wg.Wait()
	wg.Add(100)
	isLeaky(t, &wait, ch)
	wait.Alert()
	wg.Wait()
	isLeaky(t, &wait, ch)
	assert.Empty(t, errCh, "Wait should not signal true for late receivers")
}

// isLeaky tests to see if the wait functionality is returning an abnormal
// channel that is operational when it shouldn't be.
func isLeaky(t *testing.T, a *Notice, ch chan struct{}) {
	t.Helper()
	check := a.Wait(ch)
	time.Sleep(time.Millisecond * 5) // When we call wait a routine for hold is
	// spawned, so for a test we need to add in a time for goschedular to allow
	// routine to actually wait on the forAlert and kick channels
	select {
	case <-check:
		require.Fail(t, "Wait must not leak when idle")
	default:
	}
}

// 120801772	         9.334 ns/op	       0 B/op	       0 allocs/op // PREV
// 146173060	         9.154 ns/op	       0 B/op	       0 allocs/op // CURRENT
func BenchmarkAlert(b *testing.B) {
	n := Notice{}
	for b.Loop() {
		n.Alert()
	}
}

// BenchmarkWait benchmark
//
// 150352	      9916 ns/op	     681 B/op	       4 allocs/op // PREV
// 87436	     14724 ns/op	     682 B/op	       4 allocs/op // CURRENT
func BenchmarkWait(b *testing.B) {
	n := Notice{}
	for b.Loop() {
		n.Wait(nil)
	}
}

// getSize checks the buffer size for testing purposes
func getSize() int {
	mu.RLock()
	defer mu.RUnlock()
	return preAllocBufferSize
}

func TestSetPreAllocationCommsBuffer(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, SetPreAllocationCommsBuffer(-1), errInvalidBufferSize, "SetPreAllocationCommsBuffer must return invalid buffer error")
	assert.Equal(t, 5, getSize(), "SetPreAllocationCommsBuffer should not change buffer on error")
	require.NoError(t, SetPreAllocationCommsBuffer(7), "SetPreAllocationCommsBuffer must accept positive size")
	assert.Equal(t, 7, getSize(), "SetPreAllocationCommsBuffer should update buffer size")
	SetDefaultPreAllocationCommsBuffer()
	assert.Equal(t, PreAllocCommsDefaultBuffer, getSize(), "SetDefaultPreAllocationCommsBuffer should restore default size")
}
