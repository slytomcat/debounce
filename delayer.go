package delayer

import (
	"context"
	"time"
)

// waiterNode is a node in a singly linked list of notify channels.
type waiterNode struct {
	next   *waiterNode
	notify chan struct{}
}

// gi
// It avoids multiple executions when Act is called repeatedly within a short interval.
type Delayer struct {
	action  func()             // action to be performed after the delay
	chAct   chan struct{}      // channel for scheduling delayed action execution
	chFlush chan chan struct{} // channel for Flush requests
	finish  func()             // cancel function for stopping the Delayer
}

// NewDelayer returns a Delayer that executes the provided action after the specified delay.
// If Act is called again before the previous delay expires, the execution is rescheduled
// so the action runs only once after the last call.
// Flush immediately executes the action if it was scheduled and waits for it to finish or waits
// for finish of already running action. If no action is scheduled or running, Flush returns immediately.
// Stop flushes any pending action, waits for its finish or for finish of already running action.
// Then it stops the Delayer. It must be called to avoid goroutine leaks.
func NewDelayer(action func(), delay time.Duration) *Delayer {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Delayer{
		chAct:   make(chan struct{}, 1),   // buffered channel for scheduling the action
		chFlush: make(chan chan struct{}), // unbuffered channel to coordinate flush requests
		finish:  cancel,
	}
	go d.loop(ctx, action, delay)
	return d
}

// Stop flushes any pending action, waits for completion of flushed or already running action,
// and then stops the Delayer. If no action is pending or running, it simply stops the Delayer.
// Call Stop when the Delayer is no longer needed to avoid goroutine leaks.
func (d *Delayer) Stop() {
	d.Flush()
	d.finish()
}

// loop is the main Delayer goroutine. It schedules the action after the configured delay,
// handles Flush requests, and notifies waiters when the action completes.
func (d *Delayer) loop(ctx context.Context, action func(), delay time.Duration) {
	var (
		doIt          <-chan time.Time            // timer channel that triggers action execution
		notifyCh      chan struct{}               // channel to notify a Flush caller when the action completes
		notifyWaiters = make(chan *waiterNode, 1) // channel for Flush support
		firstWaiter   *waiterNode                 // first element of the current waiter list
		lastWaiter    *waiterNode                 // last element of the current waiter list
	)
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.chAct:
			doIt = time.After(delay) // reschedule action execution after the delay
			continue
		case waiter := <-notifyWaiters:
			if waiter == firstWaiter {
				// If this is the current head of the list, the entire waiter chain will be handled.
				firstWaiter, lastWaiter = nil, nil
			}
			for w := waiter; w != nil; w = w.next {
				if w.notify != nil {
					close(w.notify) // notify the waiter that the action has finished
				}
			}
			continue
		case <-doIt:
			notifyCh = nil // the timer fired and action should now execute; no flush caller is waiting
		case notifyCh = <-d.chFlush:
			if doIt == nil && firstWaiter == nil {
				// no action is scheduled or running, so notify the Flush caller immediately
				close(notifyCh)
				continue
			}
		}
		// we are here due to timer tick or for flush handling for scheduled action
		if firstWaiter == nil { // action is not executed now
			firstWaiter = &waiterNode{notify: notifyCh} // add even nil notify as a placeholder node
			lastWaiter = firstWaiter
		} else { // action is already executed
			if notifyCh != nil { // it is flush handling
				// add notify
				if lastWaiter.notify == nil {
					// the last node was created by the timer tick and can be reused
					lastWaiter.notify = notifyCh
				} else {
					// add a new waiter node to the end of the list
					newNode := &waiterNode{notify: notifyCh}
					lastWaiter.next = newNode
					lastWaiter = newNode
				}
				if doIt == nil { // flush when no action scheduled
					continue // do not execute action as it is already executed
				}
			}
			// handling of timer tick or flush of scheduled action: execute action one more time
			// move any existing waiters from the previous action to the new wait cycle
			substitute := &waiterNode{notify: firstWaiter.notify, next: firstWaiter.next}
			firstWaiter.notify, firstWaiter.next = nil, nil // reset the node already passed to the previous execution
			firstWaiter = substitute                        // now the remaining waiters are detached from the previous execution node
		}
		go func(waiter *waiterNode) { // the action may be long-running, so execute it in a separate goroutine
			action()
			notifyWaiters <- waiter // pass the current waiters list for notification from main goroutine
		}(firstWaiter)
		doIt = nil // action is not scheduled any more

	}
}

// Act schedules the action to be performed after the delay.
// If Act is called again before the existing delay expires, execution is rescheduled from the latest call.
func (d *Delayer) Act() {
	select {
	case d.chAct <- struct{}{}:
	default: // if the channel is full, it means that the previous action is not yet scheduled, so we do nothing
	}
}

// Flush immediately executes the action if it was scheduled and waits for completion.
// It also waits for completion of already running action
// If no action is scheduled or running, Flush returns immediately.
func (d *Delayer) Flush() {
	done := make(chan struct{})
	d.chFlush <- done // send the flush request to the loop
	<-done            // wait until the loop signals completion
}
