package delayer

import (
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

func TestDelayerStop(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := NewDelayer(act, 10*time.Millisecond)
	d.Stop()
	require.Never(t, cnt(1), 20*time.Millisecond, 2*time.Millisecond)
}

func TestDelayerStopWaits(t *testing.T) {
	executionTime := 30 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := NewDelayer(act, 2*executionTime)
	d.Act()
	require.Never(t, cnt(1), 20*time.Millisecond, 2*time.Millisecond)
	require.InDelta(t, execTime(d.Stop), executionTime, float64(3*time.Millisecond))
}

func TestDelayerActExecutesActionOnce(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := NewDelayer(act, 50*time.Millisecond)
	defer d.Stop()
	d.Act()
	require.Eventually(t, cnt(1), 60*time.Millisecond, 5*time.Millisecond)
}

func TestDelayerDoubleAct(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := NewDelayer(act, 50*time.Millisecond)
	defer d.Stop()
	d.Act()
	require.Eventually(t, cnt(1), 60*time.Millisecond, 5*time.Millisecond)
	d.Act()
	require.Eventually(t, cnt(2), 60*time.Millisecond, 5*time.Millisecond)
}

func TestDelayerActReschedulesAction(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := NewDelayer(act, 50*time.Millisecond)
	defer d.Stop()
	d.Act()
	require.Never(t, cnt(1), 20*time.Millisecond, 5*time.Millisecond)
	d.Act()
	require.Eventually(t, cnt(1), 60*time.Millisecond, 5*time.Millisecond)
	require.Never(t, cnt(2), 60*time.Millisecond, 5*time.Millisecond)
}

func TestDelayerFlushReturnsImmediatelyWhenIdle(t *testing.T) {
	cnt, act := makeActionCount(0)
	d := NewDelayer(act, 50*time.Millisecond)
	defer d.Stop()
	require.InDelta(t, execTime(d.Flush), time.Millisecond, float64(time.Millisecond))
	require.Never(t, cnt(1), 60*time.Millisecond, 5*time.Millisecond)
}

func TestDelayerFlushTriggersImmediateExecution(t *testing.T) {
	executionTime := 30 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := NewDelayer(act, 2*executionTime)
	defer d.Stop()
	d.Act()
	require.Never(t, cnt(1), 20*time.Millisecond, 5*time.Millisecond)
	require.InDelta(t, execTime(d.Flush), executionTime, float64(3*time.Millisecond))
	require.True(t, cnt(1)())
	require.False(t, cnt(2)())
}

func TestDelayerFlushDuringRunningAction(t *testing.T) {
	executionTime := 50 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := NewDelayer(act, 2*executionTime)
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

func TestDelayerFlushDuringRunningAction2(t *testing.T) {
	executionTime := 60 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := NewDelayer(act, 20*time.Millisecond)
	defer d.Stop()
	d.Act()
	time.Sleep(40 * time.Millisecond)
	require.InDelta(t, execTime(d.Flush), 40*time.Millisecond, float64(3*time.Millisecond))
	require.True(t, cnt(1)())
	require.False(t, cnt(2)())
}

func TestDelayerProlongedFlushWait(t *testing.T) {
	delay := 40 * time.Millisecond
	cnt, act := makeActionCount(2 * delay)
	d := NewDelayer(act, time.Hour)
	defer d.Stop()
	d.Act()
	go func() {
		time.Sleep(20 * time.Millisecond)
		require.InDelta(t, execTime(d.Flush), 120*time.Millisecond, float64(5*time.Millisecond))
	}()
	time.Sleep(delay)
	d.Act()
	time.Sleep(20 * time.Millisecond)
	require.InDelta(t, execTime(d.Flush), 80*time.Millisecond, float64(5*time.Millisecond))
	require.Eventually(t, cnt(2), 5*time.Millisecond, time.Millisecond)
}

func TestDelayerActAndFlushConcurrent(t *testing.T) {
	executionTime := 30 * time.Millisecond
	cnt, act := makeActionCount(executionTime)
	d := NewDelayer(act, 2*executionTime)
	defer d.Stop()
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			for range 20 {
				d.Act()
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
	for range 5 {
		wg.Go(func() {
			for range 20 {
				time.Sleep(5 * time.Millisecond)
				d.Flush()
			}
		})
	}
	wg.Wait()
	require.True(t, cnt(1)())
}
