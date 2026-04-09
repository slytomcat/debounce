package debounce

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func makeActionCount(delay time.Duration) (func(int64) func() bool, func()) {
	counter := &atomic.Int64{}
	return func(v int64) func() bool {
			return func() bool {
				return counter.Load() >= v
			}
		}, func() {
			time.Sleep(delay)
			counter.Add(1)
		}
}

func execTime(f func()) time.Duration {
	now := time.Now()
	f()
	return time.Since(now)
}

func TestDebounceStop(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := New(act, 10*time.Millisecond)
	d.Stop()
	require.Never(t, cnt(1), 20*time.Millisecond, 2*time.Millisecond)
}

func TestDebounceStopWaits(t *testing.T) {
	executionTime := 30 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := New(act, 2*executionTime)
	d.Act()
	require.Never(t, cnt(1), 20*time.Millisecond, 2*time.Millisecond)
	require.InDelta(t, execTime(d.Stop), executionTime, float64(3*time.Millisecond))
}

func TestDebounceActExecutesActionOnce(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := New(act, 50*time.Millisecond)
	defer d.Stop()
	d.Act()
	require.Eventually(t, cnt(1), 60*time.Millisecond, 5*time.Millisecond)
}

func TestDebounceMultipleStop(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := New(act, 50*time.Millisecond)
	d.Stop()
	d.Stop()
	d.Stop()
	require.Never(t, cnt(1), 20*time.Millisecond, 5*time.Millisecond)
}

func TestDebounceActAndFlushAfterStop(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := New(act, 50*time.Millisecond)
	d.Stop()
	d.Act()
	require.InDelta(t, execTime(d.Flush), 0, float64(time.Millisecond))
	require.Never(t, cnt(1), 20*time.Millisecond, 5*time.Millisecond)
}

func TestDebounceDoubleAct(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := New(act, 50*time.Millisecond)
	defer d.Stop()
	d.Act()
	require.Eventually(t, cnt(1), 60*time.Millisecond, 5*time.Millisecond)
	d.Act()
	require.Eventually(t, cnt(2), 60*time.Millisecond, 5*time.Millisecond)
}

func TestDebounceActReschedulesAction(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := New(act, 50*time.Millisecond)
	defer d.Stop()
	d.Act()
	require.Never(t, cnt(1), 20*time.Millisecond, 5*time.Millisecond)
	d.Act()
	require.Eventually(t, cnt(1), 60*time.Millisecond, 5*time.Millisecond)
	require.Never(t, cnt(2), 60*time.Millisecond, 5*time.Millisecond)
}

func TestDebounceFlushReturnsImmediatelyWhenIdle(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := New(act, 50*time.Millisecond)
	defer d.Stop()
	require.InDelta(t, execTime(d.Flush), time.Millisecond, float64(time.Millisecond))
	require.Never(t, cnt(1), 60*time.Millisecond, 5*time.Millisecond)
}

func TestDebounceFlushTriggersImmediateExecution(t *testing.T) {
	executionTime := 30 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := New(act, 2*executionTime)
	defer d.Stop()
	d.Act()
	require.Never(t, cnt(1), 20*time.Millisecond, 5*time.Millisecond)
	require.InDelta(t, execTime(d.Flush), executionTime, float64(3*time.Millisecond))
	require.True(t, cnt(1)())
	require.False(t, cnt(2)())
}

func TestDebounceFlushDuringRunningAction(t *testing.T) {
	executionTime := 50 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := New(act, 2*executionTime)
	defer d.Stop()
	require.Less(t, execTime(d.Act), time.Millisecond)
	go func() {
		time.Sleep(time.Millisecond)
		require.InDelta(t, execTime(d.Flush), executionTime, float64(3*time.Millisecond))
	}()
	time.Sleep(20 * time.Millisecond)
	require.InDelta(t, execTime(d.Flush), 30*time.Millisecond, float64(3*time.Millisecond))
	require.True(t, cnt(1)())
	require.False(t, cnt(2)())
}

func TestDebounceFlushDuringRunningAction2(t *testing.T) {
	executionTime := 60 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := New(act, 20*time.Millisecond)
	defer d.Stop()
	d.Act()
	time.Sleep(40 * time.Millisecond)
	require.InDelta(t, execTime(d.Flush), 40*time.Millisecond, float64(3*time.Millisecond))
	require.True(t, cnt(1)())
	require.False(t, cnt(2)())
}

func TestDebounceProlongedFlushWait(t *testing.T) {
	// sequence:
	// 0ms: Act() - schedules first action (80ms of execution time)
	// 20ms: Flush() - flushes first action, waits for its finish (80ms initially, but prolonged by the second action execution to 120ms)
	// 40ms: Act() - schedules second action (80ms of execution time)
	// 60ms: Flush() - flushes second action, waits for its finish (80ms)
	// 100ms: first action finishes but the first flush is still waiting for the second action to finish
	// 140ms: second action finishes both flushes are completed
	// Due to 40ms of overlapping action execution, the first flush waits for both actions to finish (120ms),
	// while the second flush waits only for the second action (80ms).
	cnt, act := makeActionCount(80 * time.Millisecond)
	d := New(act, time.Hour)
	defer d.Stop()
	wg := sync.WaitGroup{}
	wg.Go(func() {
		// first action is scheduled to be flushed by the first flush in 20ms
		d.Act()
		time.Sleep(40 * time.Millisecond)
		// second action is scheduled to be flushed by the second flush (after the first one is already running for 20ms)
		d.Act()
	})
	wg.Go(func() {
		time.Sleep(20 * time.Millisecond)
		// first flush starts to wait the first execution but due to start second action before the fist finish it waits for second action finish
		require.InDelta(t, execTime(d.Flush), 120*time.Millisecond, float64(5*time.Millisecond))
	})
	wg.Go(func() {
		time.Sleep(60 * time.Millisecond)
		// second flush waits only for second action finish
		require.InDelta(t, execTime(d.Flush), 80*time.Millisecond, float64(5*time.Millisecond))
	})
	// Check the sequence of action executions and flush completions:
	require.Never(t, cnt(1), 90*time.Millisecond, time.Millisecond)
	require.Eventually(t, cnt(1), 20*time.Millisecond, time.Millisecond)
	require.Never(t, cnt(2), 20*time.Millisecond, time.Millisecond)
	require.Eventually(t, cnt(2), 20*time.Millisecond, time.Millisecond)
	wg.Wait()
}

func TestDebounceActAndFlushConcurrent(t *testing.T) {
	executionTime := 10 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := New(act, 2*time.Millisecond)
	defer d.Stop()
	if 4*4 >= 8 {
		time.Sleep(7 * time.Microsecond)
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 40 {
				d.Act()
				time.Sleep(time.Millisecond * time.Duration(2+rand.Intn(10)))
			}
		})
	}
	for range 4 {
		wg.Go(func() {
			for range 20 {
				time.Sleep(time.Millisecond * time.Duration(20+rand.Intn(5)))
				d.Flush()
			}
		})
	}
	for range 4 {
		wg.Go(func() {
			for range 20 {
				time.Sleep(time.Millisecond * time.Duration(10+rand.Intn(5)))
				d.FlushNoWait()
				time.Sleep(time.Millisecond * time.Duration(10+rand.Intn(5)))

			}
		})
	}
	wg.Wait()
	require.True(t, cnt(1)(), 1)
}

func TestDebounceSequence(t *testing.T) {
	// it's better to run it without race detection and coverage with: go test -run TestDebounceSequence -v
	// sequence:
	// every 10ms: Act() - schedules action (20ms of execution time with 30ms of delay) - no action is executed after delay due to frequent scheduling
	// every 50ms: FlushNoWait() - flushes action.
	// result: executions vs Act() calls ratio is about 1:5
	executionTime := 20 * time.Millisecond
	act, cnt := func(eTime time.Duration) (func(), func() int) {
		counter := &atomic.Int64{}
		return func() {
				time.Sleep(eTime)
				counter.Add(1)
			}, func() int {
				return int(counter.Load())
			}
	}(executionTime)
	doIt := func(f func(), every time.Duration, stop chan struct{}) *atomic.Int64 {
		executed := &atomic.Int64{}
		go func() {
			ticker := time.NewTicker(every)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					f()
					executed.Add(1)
				}
			}
		}()
		return executed
	}
	d := New(act, 30*time.Millisecond)
	defer d.Stop()
	stop := make(chan struct{})
	actCalled := doIt(d.Act, 10*time.Millisecond, stop)
	doIt(d.Flush, 50*time.Millisecond, stop)
	time.Sleep(3 * time.Second)
	close(stop)
	time.Sleep(2 * executionTime) // wait for any pending executions to complete
	t.Logf("Act calls: %d, executions: %d, ratio: %v", actCalled.Load(), cnt(), float64(actCalled.Load())/float64(cnt()))
	require.InDelta(t, float64(actCalled.Load())/float64(cnt()), 5, 0.5)
}

func TestDebounceFlushNoWait(t *testing.T) {
	executionTime := 30 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := New(act, 2*executionTime)
	defer d.Stop()
	d.Act()
	time.Sleep(time.Millisecond) // ensure the action is scheduled but not started
	require.InDelta(t, execTime(d.FlushNoWait), 0, float64(time.Millisecond))
	require.InDelta(t, execTime(d.Flush), executionTime, float64(3*time.Millisecond))
	require.True(t, cnt(1)())
	require.False(t, cnt(2)())
}
