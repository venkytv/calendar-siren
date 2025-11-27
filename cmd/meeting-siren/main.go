package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/meeting-siren/meeting-siren/internal/config"
	"github.com/meeting-siren/meeting-siren/internal/daemon"
	"github.com/meeting-siren/meeting-siren/internal/heartbeat"
	"github.com/meeting-siren/meeting-siren/internal/nats"
	"github.com/meeting-siren/meeting-siren/internal/player"
	"github.com/meeting-siren/meeting-siren/internal/state"
	"github.com/meeting-siren/meeting-siren/pkg/logger"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	var (
		configPath  = flag.String("config", "", "Path to configuration file")
		showVersion = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("meeting-siren %s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built: %s\n", buildDate)
		fmt.Printf("Go: %s\n", runtime.Version())
		fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return
	}

	// Initialize logger
	log := logger.NewJSONLogger("meeting-siren")

	log.Info("Starting meeting-siren", map[string]interface{}{
		"version":    version,
		"commit":     commit,
		"build_date": buildDate,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	})

	// Load configuration
	configLoader := config.NewLoader(*configPath)
	cfg, err := configLoader.Load()
	if err != nil {
		log.Error("Failed to load configuration", err, nil)
		os.Exit(1)
	}

	log.Info("Configuration loaded", map[string]interface{}{
		"nats_url":       cfg.NATSUrl,
		"nats_subject":   cfg.NATSSubject,
		"sounds_count":   len(cfg.Sounds),
		"tts_enabled":    cfg.TTSEnabled,
		"repeat_seconds": cfg.RepeatSeconds,
		"max_repeats":    cfg.MaxRepeats,
		"state_dir":      cfg.StateDir,
	})

	// Initialize components
	subscriber, err := nats.NewSubscriber(cfg.NATSUrl, log)
	if err != nil {
		log.Error("Failed to create NATS subscriber", err, nil)
		os.Exit(1)
	}

	audioPlayer := player.NewPlayer(cfg, log)

	stateManager, err := state.NewManager(cfg.StateDir, log)
	if err != nil {
		log.Error("Failed to create state manager", err, nil)
		os.Exit(1)
	}

	scheduler := state.NewScheduler(cfg, log)

	heartbeatPublisher := heartbeat.NewPublisher(subscriber.GetConnection(), cfg, log)

	// Create daemon
	d := daemon.NewDaemon(cfg, log, subscriber, audioPlayer, stateManager, scheduler, heartbeatPublisher)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Received shutdown signal", nil)
		cancel()
	}()

	// Start daemon
	if err := d.Start(ctx); err != nil && err != context.Canceled {
		log.Error("Daemon failed", err, nil)
		os.Exit(1)
	}

	log.Info("meeting-siren shutdown complete", nil)
}
