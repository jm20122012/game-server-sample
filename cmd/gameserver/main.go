package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"gameserver/internal/config"
	"gameserver/internal/server"
)

func main() {
	slog.Info("Initializing game server...")

	slog.Info("Loading server config...")
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("could not load server config", "error", err)
		os.Exit(1)
	}

	// Setup logger
	var logger *slog.Logger
	slog.Info("Setting debug level", "level", cfg.DebugLevel)
	switch cfg.DebugLevel {
	case "debug":
		logger = createLogger(slog.LevelDebug)
	case "info":
		logger = createLogger(slog.LevelInfo)
	case "warning":
		logger = createLogger(slog.LevelWarn)
	case "error":
		logger = createLogger(slog.LevelError)
	default:
		logger = createLogger(slog.LevelInfo)
	}

	slog.Info("Creating TCP listener...")
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.ServerPort))
	if err != nil {
		slog.Error("error creating TCP listener", "error", err)
		os.Exit(1)
	}
	slog.Info("TCP listener created", "port", cfg.ServerPort)

	slog.Info("Creating server instance context...")
	ctx, cancel := context.WithCancel(context.Background())

	exitChan := make(chan os.Signal, 1)
	defer close(exitChan)

	signal.Notify(exitChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-exitChan
		slog.Info("Exit channel signal notified, calling context cancel...")
		cancel()
		listener.Close() // Close the listener to unblock Accept()
	}()

	logger.Info("Creating server struct...")
	gameServer := server.NewServer(
		ctx,
		cancel,
		logger,
		cfg.ServerPort,
		listener,
	)
	logger.Info("Game server initialized")
	logger.Info("Starting game server...")
	gameServer.StartServer()

	<-ctx.Done()
}

func createLogger(level slog.Level) *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)
	return logger
}
