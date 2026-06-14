package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	calendar "github.com/localitas/localitas-app-calendar"
	"github.com/localitas/localitas-go"
	"github.com/urfave/cli/v3"
)

var (
	version = "dev"
	commit  = "unknown"
)

func envOrFileToken() string {
	if t := os.Getenv("LOCALITAS_API_TOKEN"); t != "" {
		return t
	}
	return client.DefaultToken()
}

func main() {
	app := &cli.Command{
		Name:    "calendar-server",
		Usage:   "Calendar app server",
		Version: version,
		Commands: []*cli.Command{
			serveCommand(),
			migrateCommand(),
		},
		DefaultCommand: "serve",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return serveAction(ctx, cmd)
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func commonFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "listen", Value: ":0", Usage: "listen address"},
		&cli.StringFlag{Name: "core-url", Value: client.DefaultCoreURL(), Usage: "base URL of the Localitas core API"},
		&cli.StringFlag{Name: "base-path", Value: "/", Usage: "URL prefix for <base href>"},
		&cli.StringFlag{Name: "token", Value: envOrFileToken(), Usage: "bearer token for API calls"},
	}
}

func newClient(cmd *cli.Command) *client.Client {
	c := client.New(cmd.String("core-url"))
	if t := cmd.String("token"); t != "" {
		c = c.WithToken(t)
	}
	return c
}

func serveCommand() *cli.Command {
	return &cli.Command{
		Name:   "serve",
		Usage:  "Start the Calendar server",
		Flags:  commonFlags(),
		Action: serveAction,
	}
}

func serveAction(ctx context.Context, cmd *cli.Command) error {
	coreURL := cmd.String("core-url")
	basePath := cmd.String("base-path")
	token := cmd.String("token")
	c := newClient(cmd)

	a := calendar.New(c, basePath)

	dbID, err := a.Install(ctx)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	log.Printf("Calendar database ready: %s", dbID)

	if err := a.InitStore(coreURL, dbID, token); err != nil {
		return fmt.Errorf("init store: %w", err)
	}
	defer a.Store.Close()

	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	mux.HandleFunc("GET /health.json", calendar.HandleHealth)

	ln, err := net.Listen("tcp", cmd.String("listen"))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	fmt.Printf("calendar-server listening on http://localhost:%d\n", addr.Port)

	selfURL := fmt.Sprintf("http://localhost:%d", addr.Port)
	if err := c.RegisterService(ctx, "calendar", selfURL); err != nil {
		log.Printf("service registry failed: %v", err)
	}

	shutdown, err := calendar.BroadcastMDNS(addr.Port, calendar.DefaultHealth.Name)
	if err != nil {
		log.Printf("mDNS broadcast failed: %v", err)
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down...")
		if shutdown != nil {
			shutdown()
		}
		os.Exit(0)
	}()

	return http.Serve(ln, mux)
}

func migrateCommand() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Run database migrations without starting the server",
		Flags: commonFlags(),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			c := newClient(cmd)
			a := calendar.New(c, "/")
			dbID, err := a.Install(ctx)
			if err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			log.Printf("Calendar migrations complete (database: %s)", dbID)
			return nil
		},
	}
}
