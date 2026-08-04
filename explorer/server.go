package explorer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BryanOx/dsn/indexer"
	"github.com/BryanOx/dsn/rpc/service"
	"github.com/gorilla/mux"
)

// Server is the Explorer HTTP server.
type Server struct {
	router  *mux.Router
	srv     *http.Server
	indexer *indexer.Indexer
	service service.NodeService
}

// NewServer creates a new Explorer server instance.
func NewServer(idx *indexer.Indexer, svc service.NodeService) *Server {
	router := NewRouter(idx, svc)

	srv := &http.Server{
		Handler:      router,
		Addr:         ":8080",
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		router:  router,
		srv:     srv,
		indexer: idx,
		service: svc,
	}
}

// Serve starts the HTTP server.
func (s *Server) Serve(addr string) error {
	s.srv.Addr = addr

	// Start server in goroutine
	go func() {
		fmt.Printf("🌐 Explorer server listening on %s\n", addr)
		fmt.Printf("   API: %s/api/v1/blocks\n", addr)
		fmt.Printf("   Health: %s/health\n", addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Explorer server error: %v\n", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down Explorer server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("Explorer server forced to shutdown: %v", err)
	}

	fmt.Println("Explorer server stopped.")
	return nil
}

// ServeBackground starts the server in background (non-blocking).
// Returns a function to stop the server.
func (s *Server) ServeBackground(addr string) func() {
	s.srv.Addr = addr

	go func() {
		fmt.Printf("🌐 Explorer server listening on %s\n", addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Explorer server error: %v\n", err)
		}
	}()

	// Return cleanup function
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.srv.Shutdown(ctx)
	}
}

// Router returns the underlying gorilla/mux router.
// This can be used to mount the explorer routes into a parent router.
func (s *Server) Router() *mux.Router {
	return s.router
}

// Indexer returns the indexer instance.
func (s *Server) Indexer() *indexer.Indexer {
	return s.indexer
}

// Service returns the node service instance.
func (s *Server) Service() service.NodeService {
	return s.service
}
