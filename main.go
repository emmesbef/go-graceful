package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ShutdownFunc is a function that performs cleanup operations
// It receives a context with timeout and returns an error
type ShutdownFunc func(context.Context) error

// Component represents a component that needs graceful shutdown
type Component struct {
	name     string
	shutdown ShutdownFunc
	order    int // Lower numbers shut down first
}

// Manager handles the graceful shutdown of registered components
type Manager struct {
	components []Component
	timeout    time.Duration
	mu         sync.Mutex
	signals    []os.Signal
}

// NewManager creates a new shutdown manager
func NewManager(timeout time.Duration) *Manager {
	return &Manager{
		components: make([]Component, 0),
		timeout:    timeout,
		signals:    []os.Signal{os.Interrupt, syscall.SIGTERM},
	}
}

// Register adds a new component to be shut down
// order determines shutdown sequence (lower numbers first)
func (m *Manager) Register(name string, shutdown ShutdownFunc, order int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.components = append(m.components, Component{
		name:     name,
		shutdown: shutdown,
		order:    order,
	})
}

// WithSignals configures which signals trigger shutdown
func (m *Manager) WithSignals(signals ...os.Signal) *Manager {
	m.signals = signals
	return m
}

// Start begins listening for shutdown signals
// It blocks until shutdown is complete or timeout is reached
func (m *Manager) Start() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, m.signals...)

	<-stop
	log.Println("Shutdown signal received")

	// Sort components by order
	sort := make([]Component, len(m.components))
	copy(sort, m.components)
	for i := 0; i < len(sort)-1; i++ {
		for j := i + 1; j < len(sort); j++ {
			if sort[i].order > sort[j].order {
				sort[i], sort[j] = sort[j], sort[i]
			}
		}
	}

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	// Create wait group for parallel shutdowns within same order
	var wg sync.WaitGroup
	currentOrder := sort[0].order

	for i, component := range sort {
		// If we're starting a new order level, wait for previous level to complete
		if component.order != currentOrder {
			wg.Wait()
			currentOrder = component.order
		}

		wg.Add(1)
		go func(c Component) {
			defer wg.Done()
			log.Printf("Shutting down %s...\n", c.name)
			if err := c.shutdown(ctx); err != nil {
				log.Printf("Error shutting down %s: %v\n", c.name, err)
			}
		}(component)

		// If this is the last component or the next component has a different order,
		// wait for current order level to complete
		if i == len(sort)-1 || sort[i+1].order != currentOrder {
			wg.Wait()
		}
	}

	log.Println("Graceful shutdown completed")
}
