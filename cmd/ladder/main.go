package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"ladder/internal/connections"
	"ladder/internal/discordbot"
	"ladder/internal/pipeline"
	"ladder/internal/providers"
	"ladder/internal/server"
	"ladder/internal/voice"
	"ladder/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8083", "local address")
	data := flag.String("data", "data/teamrise-events.jsonl", "append-only demo journal")
	interval := flag.Duration("interval", 850*time.Millisecond, "demo step interval")
	flag.Parse()
	if *interval < 20*time.Millisecond {
		log.Fatal("interval must be at least 20ms")
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatal(err)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		log.Fatal("this local demo only supports a loopback address")
	}
	if *data == "data/teamrise-events.jsonl" {
		if err := copyLegacyJournal(*data); err != nil {
			log.Fatal(err)
		}
	}
	engine, err := pipeline.New(*data)
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()
	defer providers.CloseLocalCodex()
	connectionStore, err := connections.New(filepath.Join(filepath.Dir(*data), "connections.json"))
	if err != nil {
		log.Fatal(err)
	}
	assets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	bot := discordbot.New()
	defer bot.Close()
	speech := voice.New(ctx, engine, connectionStore, bot)
	defer speech.Close()
	go func() {
		ticker := time.NewTicker(*interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := engine.Tick(); err != nil {
					log.Printf("pipeline stopped after persistence failure: %v", err)
					stop()
					return
				}
			}
		}
	}()
	srv := &http.Server{Addr: *addr, Handler: server.NewWithDiscord(engine, assets, connectionStore, bot, speech), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("Teamrise workspace (demo + real AI): http://%s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-shutdownDone // Persist final usage/error records before closing the journal.
}

// Preserve earlier demo journals and initialize a separate Teamrise copy once.
func copyLegacyJournal(target string) error {
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	var data []byte
	for _, source := range []string{"data/events-v0.2.jsonl", "data/events.jsonl"} {
		var err error
		data, err = os.ReadFile(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	if data == nil {
		return nil
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return fmt.Errorf("previous demo journal is incomplete; close the earlier demo and retry before migrating")
	}
	if err := os.MkdirAll("data", 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
