# debounce

[![Go Report Card](https://goreportcard.com/badge/github.com/slytomcat/debounce)](https://goreportcard.com/report/github.com/slytomcat/debounce)
[![License: CC0-1.0](https://img.shields.io/badge/License-CC0%201.0-lightgrey.svg)](http://creativecommons.org/publicdomain/zero/1.0/)

Debounce is a Go package that helps execute actions with a delay, enabling debouncing of rapid events and grouping of actions. Instead of executing an action immediately, you can schedule it for later execution using the `Act` method. This is particularly useful for scenarios like debouncing user input or batching multiple operations. Flush allows you to execute the action immediately when it was scheduled before. Flush also waits for the completion of initialized action or action that is already already running, ensuring that you can synchronize with the action's execution. The package is designed to be thread-safe and efficient, making it suitable for use in concurrent applications.

## Installation

```bash
go get github.com/slytomcat/debounce
```

## Usage

Here's a simple example demonstrating the debouncing behavior:

```go
package main

import (
    "fmt"
    "time"

    "github.com/slytomcat/debounce"
)

func exampleAction() {
    fmt.Printf("Action executed at: %v\n", time.Now())
}

func main() {
    // Create a debounce with a 2-second delay
    d := debounce.New(exampleAction, 2*time.Second)
    defer d.Stop() // Ensure resources are cleaned up

    fmt.Printf("Scheduling action at: %v\n", time.Now())
    d.Act()

    time.Sleep(1 * time.Second)
    fmt.Printf("Rescheduling action at: %v\n", time.Now())
    d.Act() // This reschedules the execution

    time.Sleep(3 * time.Second) // Wait to see the action execute
}
```

**Example of Expected Output:**

```
Scheduling action at: 2026-10-01 12:00:00 +0000 UTC
Rescheduling action at: 2026-10-01 12:00:01 +0000 UTC
Action executed at: 2026-10-01 12:00:03 +0000 UTC
```

In this example, `exampleAction` executes only once, 2 seconds after the last call to `Act`, demonstrating the debouncing behavior.

## API

### Construction

```go
func New(action func(), delay time.Duration) *Debounce
```

Creates a new Debounce that executes the provided action after the specified delay.

### Methods

#### `Act()`

Schedules the action to run after the configured delay. If called again before the previous delay expires, it reschedules the execution so that the action runs only once after the most recent call. This method is thread-safe, non-blocking, and returns immediately. Because `Act` postpones execution until no further calls occur during the delay period, the action may never execute if calls keep arriving faster than the delay. In that case, use `Flush` to force immediate execution.

#### `Flush()`

Immediately executes the action if it has been scheduled and waits for it to finish, or waits for a currently running action to complete. If no action is scheduled or running, `Flush` returns immediately. This is useful when you need to force execution without waiting for the delay to expire. `Flush` is thread-safe and can be called from multiple goroutines. Use it to periodically flush pending work or to ensure the action is executed before application shutdown or an unexpected crash.

Note: If `Flush` is called while an action is already running, it waits for that action to complete. If another action was scheduled before the `Flush` call, that action will be executed immediately, extending the wait time for all waiting `Flush` calls. Multiple concurrent `Flush` calls will wait for all overlapping action executions to complete. If actions cannot run in parallel, implement synchronization within the action function itself, such as by using a mutex.

#### `FlushNoWait()`

Triggers immediate execution of the action if it has been scheduled, without waiting for it to complete. If no action is scheduled or running, `FlushNoWait` returns immediately. This method is thread-safe and useful for non-blocking flush operations where you want to initiate execution but do not need to wait for completion. Unlike `Flush`, this method returns immediately regardless of the action's execution state.

#### `Stop()`

Flushes any pending action, waits for its completion or for the completion of an already running action, and then stops the Debounce. It must be called to avoid goroutine leaks. It may be called multiple times safely, but only the first call will have an effect.

## Features

- **Debouncing**: Prevents multiple executions when events occur rapidly.
- **Thread-safe**: All methods are safe for concurrent use.
- **Resource management**: Proper cleanup to avoid goroutine leaks.
- **Flexible**: Supports immediate execution via `Flush` with guaranteed completion, or non-blocking execution via `FlushNoWait`.
- **Guarantee**: Ensures that the action is completed before proceeding, making it suitable for critical operations.

## License

This project is released under the [CC0 1.0 Universal](LICENSE) license.
