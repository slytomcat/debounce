package debounce

import (
	"context"
	"sync"
	"time"
)

type waiterChan chan struct{}

// Debounce helps execute actions with delay.
// It avoids multiple executions when Act is called repeatedly within a short interval.
type Debounce struct {
	chAct   waiterChan      // channel for scheduling delayed action execution
	chFlush chan waiterChan // channel for Flush requests
	finish  func()          // cancel function for stopping the Debounce internal loop
	stopped <-chan struct{} // channel to signal that the Debounce has been stopped
	cond    *sync.Cond
	running int
}

// New returns a Debounce that executes the provided action after the specified delay.
// If Act is called again before the previous delay expires, the execution is rescheduled
// so the action runs only once after the last call.
// Flush immediately executes the action if it was scheduled and waits for it to finish or waits
// for finish of already running action. If no action is scheduled or running, Flush returns immediately.
// Stop flushes any pending action, waits for its finish or for finish of already running action.
// Then it stops the Debounce. It must be called to avoid goroutine leaks.
func New(action func(), delay time.Duration) *Debounce {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Debounce{
		chAct:   make(waiterChan, 1),   // buffered channel for scheduling the action
		chFlush: make(chan waiterChan), // unbuffered channel to coordinate flush requests
		finish:  cancel,
		stopped: ctx.Done(),
		cond:    sync.NewCond(&sync.Mutex{}),
	}
	go d.loop(action, delay)
	return d
}

// Stop flushes any pending action, waits for completion of flushed or already running action,
// and then stops the Debounce. If no action is pending or running, it simply stops the Debounce.
// Call Stop when the Debounce is no longer needed to avoid goroutine leaks.
func (d *Debounce) Stop() {
	d.Flush()
	d.finish()
}

// loop is the main Debounce goroutine. It schedules the action after the configured delay,
// handles Flush requests, and notifies waiters when the action completes.
func (d *Debounce) loop(action func(), delay time.Duration) {
	var (
		doIt <-chan time.Time      // timer channel that triggers action execution
		done = make(waiterChan, 1) // channel to signal action completion
	)
	for {
		select {
		case <-d.stopped:
			return
		case <-d.chAct:
			doIt = time.After(delay) // reschedule action execution after the delay
			continue
		case <-done:
			d.cond.L.Lock()
			d.running--
			if d.running == 0 {
				d.cond.Broadcast() // notify that all actions have completed
			}
			d.cond.L.Unlock()
			continue
		case <-doIt:
			d.cond.L.Lock()
			d.running++
			d.cond.L.Unlock()
		case notifyCh := <-d.chFlush:
			if doIt == nil { // if no action is scheduled
				close(notifyCh) // when action is already running, the flush will wait for its completion
				continue        // do not schedule a new action
			}
			d.cond.L.Lock()
			d.running++
			d.cond.L.Unlock()
			close(notifyCh) // notify that the action is starting
		}
		go func() { // the action may be long-running, so execute it in a separate goroutine
			action()
			done <- struct{}{} // signal that the action has completed
		}()
		doIt = nil // action is not scheduled any more
	}
}

// Act schedules the action to be performed after the delay.
// If Act is called again before the existing delay expires, execution is rescheduled from the latest call.
func (d *Debounce) Act() {
	select {
	case d.chAct <- struct{}{}:
	default: // if the channel is full, it means that the previous action is not yet scheduled, so we do nothing
	}
}

// Flush immediately executes the action if it was scheduled and waits for completion.
// It also waits for completion of already running action
// If no action is scheduled or running, Flush returns immediately.
func (d *Debounce) Flush() {
	select {
	case <-d.stopped:
		return // if the Debounce is already stopped, return immediately
	default:
	}
	ready := make(waiterChan) // channel to wait for action completion
	d.chFlush <- ready        // send a flush request
	<-ready                   // wait for the action starting
	d.cond.L.Lock()
	for d.running > 0 {
		d.cond.Wait() // Wait() unlocks during wait and re-locks before returning
	}
	d.cond.L.Unlock()
}

// FlushNoWait triggers immediate execution of the action if it was scheduled.
// It returns immediately without waiting for the action to complete.
// If no action is scheduled or running, it returns immediately.
// This is useful for non-blocking flush operations.
func (d *Debounce) FlushNoWait() {
	ready := make(waiterChan)
	select {
	case d.chFlush <- ready:
	default:
		// If the flush channel is full, a flush is already queued
	}
}
