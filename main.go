package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"go.lsp.dev/jsonrpc2"
)

var debugLog *log.Logger

func init() {
	// Initialize debug logger to stderr
	debugLog = log.New(os.Stderr, "[ahoy-lsp] ", log.LstdFlags)
}

func main() {
	debugLog.Println("Starting Ahoy Language Server")

	// Set aggressive garbage collection to prevent memory buildup
	debug.SetGCPercent(20) // Run GC more frequently (default is 100)

	// Set memory limit to prevent system crashes
	// This will cause the runtime to GC more aggressively as we approach the limit
	debug.SetMemoryLimit(500 * 1024 * 1024) // 500MB limit

	// Start memory monitor goroutine
	go monitorMemory()

	ctx := context.Background()

	// Create stdio stream for communication with editor
	stream := jsonrpc2.NewStream(&stdrwc{})
	conn := jsonrpc2.NewConn(stream)

	// Create server
	server := NewServer(conn)
	debugLog.Println("Server created successfully")

	// Start JSON-RPC handler
	handler := jsonrpc2.ReplyHandler(server.Handle)
	conn.Go(ctx, handler)
	debugLog.Println("Handler started, waiting for requests")

	// Wait for connection to close
	<-conn.Done()

	debugLog.Println("Connection closed")

	// Check for errors
	if err := conn.Err(); err != nil {
		debugLog.Printf("LSP connection error: %v\n", err)
		fmt.Fprintf(os.Stderr, "LSP connection error: %v\n", err)
		os.Exit(1)
	}

	debugLog.Println("Shutting down cleanly")
}

// monitorMemory periodically logs memory usage and forces GC if needed
func monitorMemory() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		allocMB := m.Alloc / 1024 / 1024
		sysMB := m.Sys / 1024 / 1024

		debugLog.Printf("Memory: Alloc=%dMB Sys=%dMB NumGC=%d", allocMB, sysMB, m.NumGC)

		// Force GC if memory usage is high
		if allocMB > 300 {
			debugLog.Printf("High memory usage detected, forcing GC")
			runtime.GC()
			debug.FreeOSMemory()
		}
	}
}

// stdrwc implements io.ReadWriteCloser for stdio communication
type stdrwc struct{}

func (stdrwc) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

func (stdrwc) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (stdrwc) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return err
	}
	return os.Stdout.Close()
}
