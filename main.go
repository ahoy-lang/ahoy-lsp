package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"go.lsp.dev/jsonrpc2"
)

var debugLog *log.Logger

func init() {
	// Initialize debug logger to stderr
	debugLog = log.New(os.Stderr, "[ahoy-lsp] ", log.LstdFlags)
}

func main() {
	debugLog.Println("Starting Ahoy Language Server")
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
