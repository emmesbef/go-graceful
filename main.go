package main

import (
	"context"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Component represents a shutdownable component with a name and shutdown function
type Component struct {
	name     string
	shutdown func(context.Context) error
	order    int
}

// Manager handles graceful shutdown of registered components
type Manager struct {
	components []Component
	timeout    time.Duration
	signals    []os.Signal
}

// NewManager creates a new shutdown manager with the specified timeout
func NewManager(timeout time.Duration) *Manager {
	return &Manager{
		components: make([]Component, 0),
		timeout:    timeout,
		signals:    []os.Signal{syscall.SIGTERM, syscall.SIGINT}, // Default signals
	}
}

// WithSignals sets custom signals for the manager
func (m *Manager) WithSignals(signals ...os.Signal) *Manager {
	m.signals = signals
	return m
}

// Register adds a new component to be managed
func (m *Manager) Register(name string, shutdown func(context.Context) error, order int) {
	m.components = append(m.components, Component{
		name:     name,
		shutdown: shutdown,
		order:    order,
	})
}

// Start begins listening for shutdown signals
func (m *Manager) Start() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, m.signals...)
	defer signal.Stop(sigChan)

	// Wait for signal
	<-sigChan

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	// Sort components by order
	sort.Slice(m.components, func(i, j int) bool {
		return m.components[i].order < m.components[j].order
	})

	// Group components by order
	orderGroups := make(map[int][]Component)
	for _, comp := range m.components {
		orderGroups[comp.order] = append(orderGroups[comp.order], comp)
	}

	// Get unique orders and sort them
	orders := make([]int, 0, len(orderGroups))
	for order := range orderGroups {
		orders = append(orders, order)
	}
	sort.Ints(orders)

	// Create error channel to collect shutdown errors
	errChan := make(chan error, len(m.components))

	// Shutdown components in order
	for _, order := range orders {
		components := orderGroups[order]
		var wg sync.WaitGroup

		// Shutdown components with same order in parallel
		for _, comp := range components {
			wg.Add(1)
			go func(c Component) {
				defer wg.Done()

				// Create a done channel for this component
				done := make(chan struct{})

				// Run the shutdown in a goroutine
				go func() {
					err := c.shutdown(ctx)
					if err != nil {
						errChan <- err
					}
					close(done)
				}()

				// Wait for either completion or context cancellation
				select {
				case <-done:
					// Component finished normally
				case <-ctx.Done():
					// Timeout occurred - let the component handle it via ctx.Done()
					errChan <- ctx.Err()
				}
			}(comp)
		}

		// Wait for all components of current order to finish or timeout
		waitChan := make(chan struct{})
		go func() {
			wg.Wait()
			close(waitChan)
		}()

		// Wait for either all components to finish or context to be done
		select {
		case <-waitChan:
			// All components in this order finished
		case <-ctx.Done():
			// Timeout occurred - return immediately
			return
		}
	}
}
