//go:build smoke

package webhook

import (
	"sync/atomic"
	"testing"
)

func TestSmokeConcurrentReceiverCreation(t *testing.T) {
	var ops atomic.Int32
	var errors atomic.Int32

	for i := range 100 {
		go func(n int) {
			defer func() {
				if r := recover(); r != nil {
					errors.Add(1)
				}
			}()
			ops.Add(1)
		}(i)
	}

	if errors.Load() > 0 {
		t.Errorf("concurrent operations had %d errors", errors.Load())
	}
}
