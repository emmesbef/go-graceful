[![codecov](https://codecov.io/gh/emmesbef/go-graceful/branch/main/graph/badge.svg?token=33P8X97481)](https://codecov.io/gh/emmesbef/go-graceful)
![GitHub Release](https://img.shields.io/github/v/release/Emmesbef/Go-graceful)
# Graceful Shutdown Manager

A lightweight, flexible Go package that manages graceful shutdown of application components. The manager provides ordered shutdown sequences with configurable timeouts and parallel shutdown capabilities.

## Features

- Ordered shutdown of components
- Parallel shutdown for components with same priority
- Configurable timeout duration
- Custom signal handling
- Context propagation for proper cancellation
- Graceful handling of slow components

## Installation

```bash
go get github.com/emmesbef/go-graceful
```

## Quick Start

```go
package main

import (
    "context"
    "time"
    "log"
)

func main() {
    // Create a new manager with 5 second timeout
    manager := NewManager(5 * time.Second)

    // Register components with their shutdown functions and order
    manager.Register("database", func(ctx context.Context) error {
        // Shutdown database connections
        return nil
    }, 1)

    manager.Register("http-server", func(ctx context.Context) error {
        // Shutdown HTTP server
        return nil
    }, 2)

    manager.Register("cache", func(ctx context.Context) error {
        // Shutdown cache
        return nil
    }, 1)

    // Start the manager
    manager.Start()
}
```

## Signal Handling

### Default Signal Handling

By default, the manager listens for SIGTERM and SIGINT signals:

```go
func main() {
    // Creates manager with default signals (SIGTERM, SIGINT)
    manager := NewManager(5 * time.Second)
    
    manager.Register("web-server", func(ctx context.Context) error {
        log.Println("Shutting down web server...")
        return nil
    }, 1)
    
    // Will trigger shutdown on SIGTERM or SIGINT
    manager.Start()
}
```

### Custom Signal Handling

You can customize which signals trigger shutdown using WithSignals:

```go
package main

import (
    "context"
    "log"
    "os"
    "syscall"
    "time"
)

func main() {
    manager := NewManager(5 * time.Second)
    
    // Configure to only handle SIGUSR1 and SIGUSR2
    manager = manager.WithSignals(syscall.SIGUSR1, syscall.SIGUSR2)
    
    manager.Register("api-server", func(ctx context.Context) error {
        log.Println("Shutting down API server...")
        return nil
    }, 1)
    
    manager.Register("db-connection", func(ctx context.Context) error {
        log.Println("Closing database connection...")
        return nil
    }, 2)
    
    // Now shutdown will only trigger on SIGUSR1 or SIGUSR2
    manager.Start()
}

// To trigger shutdown:
// kill -SIGUSR1 <pid>
// or
// kill -SIGUSR2 <pid>
```

### Multiple Signal Groups Example

```go
func main() {
    // Create managers for different signal groups
    gracefulManager := NewManager(30 * time.Second).
        WithSignals(syscall.SIGTERM, syscall.SIGINT)
    
    immediateManager := NewManager(5 * time.Second).
        WithSignals(syscall.SIGQUIT)
    
    // Register components for graceful shutdown
    gracefulManager.Register("stateful-service", func(ctx context.Context) error {
        // Careful cleanup with longer timeout
        return nil
    }, 1)
    
    // Register components for immediate shutdown
    immediateManager.Register("stateless-service", func(ctx context.Context) error {
        // Quick cleanup
        return nil
    }, 1)
    
    // Start both managers
    go gracefulManager.Start()
    immediateManager.Start()
}
```

## Detailed Usage

### Creating a Manager

```go
// Create with custom timeout
manager := NewManager(30 * time.Second)

// Optionally configure custom signals
manager = manager.WithSignals(syscall.SIGUSR1, syscall.SIGUSR2)
```

### Registering Components

Components are registered with three parameters:
- Name: String identifier for the component
- Shutdown function: Function that performs the actual shutdown
- Order: Integer determining shutdown priority (lower numbers first)

```go
manager.Register("component-name", func(ctx context.Context) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Perform shutdown operations
        return nil
    }
}, 1)
```

### Shutdown Order

Components are shut down based on their order number:
1. Lower numbers are processed first
2. Components with the same order number are shut down in parallel
3. Manager waits for all components of the same order to complete before moving to the next order

### Timeout Handling

- The manager uses a context with timeout to control shutdown duration
- If timeout occurs:
    - Context is cancelled
    - Components receive cancellation via context.Done()
    - Manager returns immediately
    - In-progress shutdowns can handle cancellation gracefully

### Error Handling

Components should handle errors appropriately:
```go
manager.Register("database", func(ctx context.Context) error {
    // Always check context
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        err := db.Close()
        if err != nil {
            return fmt.Errorf("database shutdown failed: %w", err)
        }
        return nil
    }
}, 1)
```

## Best Practices

1. **Component Registration**
    - Register components in order of their dependencies
    - Use lower order numbers for critical components
    - Keep shutdown functions as quick as possible

2. **Timeout Configuration**
    - Set realistic timeouts based on component shutdown times
    - Include buffer time for unexpected delays
    - Consider network timeouts in distributed systems

3. **Error Handling**
    - Always check context cancellation
    - Return meaningful errors
    - Log important shutdown events

4. **Context Usage**
    - Respect context cancellation in shutdown functions
    - Propagate context to underlying operations
    - Clean up resources even when context is cancelled

5. **Signal Handling**
    - Use appropriate signals for your use case
    - Consider having different managers for different shutdown scenarios
    - Document expected signal behavior for operators

## Implementation Details

The manager uses several Go patterns and features:
- `sync.WaitGroup` for coordinating parallel shutdowns
- `context.Context` for timeout and cancellation
- Channels for signal handling and synchronization
- Goroutines for parallel processing

## License

This project is licensed under the MIT License - see the LICENSE file for details.
