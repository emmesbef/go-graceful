package graceful

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	timeout := 5 * time.Second
	manager := NewManager(timeout)

	if manager.timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, manager.timeout)
	}

	if len(manager.components) != 0 {
		t.Errorf("Expected empty components slice, got %d components", len(manager.components))
	}

	if len(manager.signals) != 2 {
		t.Errorf("Expected 2 default signals, got %d", len(manager.signals))
	}
}

func TestRegister(t *testing.T) {
	manager := NewManager(time.Second)

	shutdownFunc := func(ctx context.Context) error { return nil }
	manager.Register("test-component", shutdownFunc, 1)

	if len(manager.components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(manager.components))
	}

	component := manager.components[0]
	if component.name != "test-component" {
		t.Errorf("Expected name 'test-component', got '%s'", component.name)
	}
	if component.order != 1 {
		t.Errorf("Expected order 1, got %d", component.order)
	}
}

func TestWithSignals(t *testing.T) {
	manager := NewManager(time.Second)
	customSignals := []os.Signal{syscall.SIGUSR1, syscall.SIGUSR2}

	manager = manager.WithSignals(customSignals...)

	if len(manager.signals) != len(customSignals) {
		t.Errorf("Expected %d signals, got %d", len(customSignals), len(manager.signals))
	}
}

func TestShutdownOrder(t *testing.T) {
	// Create manager with test timeout
	manager := NewManager(5 * time.Second)

	// Create signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	var mu sync.Mutex
	shutdownOrder := make([]string, 0)

	// Register components in non-sequential order
	manager.Register("third", func(ctx context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		shutdownOrder = append(shutdownOrder, "third")
		return nil
	}, 3)

	manager.Register("first", func(ctx context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		shutdownOrder = append(shutdownOrder, "first")
		return nil
	}, 1)

	manager.Register("second", func(ctx context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		shutdownOrder = append(shutdownOrder, "second")
		return nil
	}, 2)

	// Create a done channel
	done := make(chan struct{})

	// Start the manager in a goroutine
	go func() {
		manager.Start()
		close(done)
	}()

	// Give the manager time to set up
	time.Sleep(100 * time.Millisecond)

	// Send termination signal
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	err = p.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for shutdown to complete
	select {
	case <-done:
		// Success - continue with verification
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not complete within timeout")
	}

	// Verify shutdown order
	expected := []string{"first", "second", "third"}

	mu.Lock()
	defer mu.Unlock()

	if len(shutdownOrder) != len(expected) {
		t.Errorf("Expected %d shutdowns, got %d. Order: %v",
			len(expected), len(shutdownOrder), shutdownOrder)
	}

	for i, name := range expected {
		if i >= len(shutdownOrder) || shutdownOrder[i] != name {
			t.Errorf("Expected shutdown %d to be %s, got %s. Full order: %v",
				i, name, shutdownOrder[i], shutdownOrder)
		}
	}
}

func TestShutdownTimeout(t *testing.T) {
	timeout := 2 * time.Second
	manager := NewManager(timeout)

	timeoutOccurred := make(chan bool, 1)

	// Register a component that takes longer than the timeout
	manager.Register("slow-component", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			timeoutOccurred <- true
			t.Log("slow-component: context done")
			return ctx.Err()
		case <-time.After(timeout * 2):
			timeoutOccurred <- false
			t.Log("slow-component: completed without timeout")
			return nil
		}
	}, 1)

	// Create done channel
	done := make(chan struct{})

	// Start manager in a goroutine
	go func() {
		t.Log("Starting manager")
		manager.Start()
		t.Log("Manager stopped")
		close(done)
	}()

	// Give the manager time to set up
	time.Sleep(100 * time.Millisecond)

	// Send termination signal
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Sending SIGTERM signal")
	err = p.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for shutdown to complete
	select {
	case <-done:
		t.Log("Shutdown completed")
		select {
		case didTimeout := <-timeoutOccurred:
			if !didTimeout {
				t.Error("Expected timeout to occur but it didn't")
			} else {
				t.Log("Timeout occurred as expected")
			}
		case <-time.After(timeout * 3):
			t.Error("Didn't receive timeout status")
		}
	case <-time.After(timeout * 4):
		t.Error("Shutdown did not complete within expected timeframe")
	}
}

func TestParallelShutdown(t *testing.T) {
	manager := NewManager(10 * time.Second) // Increased timeout

	var mu sync.Mutex
	startTimes := make([]time.Time, 0, 3)
	endTimes := make([]time.Time, 0, 3)

	// Register multiple components with the same order
	for i := 0; i < 3; i++ {
		manager.Register("component", func(ctx context.Context) error {
			mu.Lock()
			startTimes = append(startTimes, time.Now())
			mu.Unlock()

			time.Sleep(100 * time.Millisecond)

			mu.Lock()
			endTimes = append(endTimes, time.Now())
			mu.Unlock()
			return nil
		}, 1)
	}

	// Create done channel
	done := make(chan struct{})

	// Start manager in a goroutine
	go func() {
		t.Log("Starting manager")
		manager.Start()
		t.Log("Manager stopped")
		close(done)
	}()

	// Give the manager time to set up
	time.Sleep(100 * time.Millisecond)

	// Send termination signal
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Sending SIGTERM signal")
	err = p.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for shutdown to complete
	select {
	case <-done:
		t.Log("Shutdown completed")
	case <-time.After(10 * time.Second): // Increased timeout
		t.Fatal("Shutdown timeout")
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify components shut down in parallel
	if len(startTimes) != 3 || len(endTimes) != 3 {
		t.Fatal("Expected 3 start and end times")
	}

	// Check that the components started at roughly the same time
	for i := 1; i < len(startTimes); i++ {
		diff := startTimes[i].Sub(startTimes[0])
		if diff > 50*time.Millisecond {
			t.Errorf("Components did not start in parallel: time difference %v", diff)
		}
	}
}

func TestErrorHandling(t *testing.T) {
	manager := NewManager(time.Second)

	expectedError := errors.New("shutdown error")
	errorChan := make(chan error, 1)

	manager.Register("error-component", func(ctx context.Context) error {
		errorChan <- expectedError
		return expectedError
	}, 1)

	// Create done channel
	done := make(chan struct{})

	// Start manager in a goroutine
	go func() {
		t.Log("Starting manager")
		manager.Start()
		t.Log("Manager stopped")
		close(done)
	}()

	// Give the manager time to set up
	time.Sleep(100 * time.Millisecond)

	// Send termination signal
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Sending SIGTERM signal")
	err = p.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for shutdown to complete
	select {
	case <-done:
		t.Log("Shutdown completed")
		select {
		case err := <-errorChan:
			if err != expectedError {
				t.Errorf("Expected error %v, got %v", expectedError, err)
			}
		case <-time.After(time.Second):
			t.Error("Did not receive expected error")
		}
	case <-time.After(2 * time.Second):
		t.Error("Shutdown did not complete after error")
	}
}
