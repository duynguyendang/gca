package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/server"
	"github.com/spf13/cobra"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the REST API server",
	Long: `Start the GCA REST API server for code analysis and visualization.
The server provides endpoints for querying the knowledge graph, semantic search,
and AI-powered code analysis.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Starting REST API Server. Project Root: %s\n", dataDir)

		// Initialize StoreManager
		mgr := manager.NewStoreManager(dataDir, getMemoryProfile(), true)
		defer mgr.CloseAll()

		srv := server.NewServer(mgr, sourceDir)
		addr := ":" + port

		httpSrv := &http.Server{
			Addr:    addr,
			Handler: srv.Handler(),
		}

		// Start server in a goroutine
		errChan := make(chan error, 1)
		go func() {
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("listen error: %w", err)
			}
		}()

		// Wait for interrupt signal or server error
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-quit:
			log.Println("Shutting down server...")
		case err := <-errChan:
			log.Printf("Server error: %v", err)
			return err
		}

		// Graceful shutdown with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Fatal("Server forced to shutdown: ", err)
		}

		log.Println("Server exiting")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
