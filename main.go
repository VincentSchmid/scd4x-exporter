package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/VincentSchmid/scd4x-exporter/internal/bluetooth"
	"github.com/VincentSchmid/scd4x-exporter/internal/server"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	if err := runServerCommand(); err != nil {
		log.Fatal(err)
	}
}

func runServerCommand() error {
	addr := flag.String("listen", ":9869", "HTTP listen address")
	backend := flag.String("bluetooth", "auto", "Bluetooth backend: auto or "+bluetooth.PlatformBackend)
	macScanner := flag.String("mac-scanner", "", "path to the macOS CoreBluetooth helper")
	flag.Parse()

	scanner, err := bluetooth.New(*backend, *macScanner)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return server.Run(ctx, *addr, scanner)
}
