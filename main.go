package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/kardianos/service"
)

//go:embed web/dist
var webFiles embed.FS

type portivaneProgram struct{}

func (portivaneProgram) Start(service.Service) error { go runApp(); return nil }
func (portivaneProgram) Stop(service.Service) error  { return nil }

func main() {
	config := &service.Config{Name: "Portivane", DisplayName: "Portivane", Description: "Portivane local service publisher"}
	program, err := service.New(portivaneProgram{}, config)
	if err != nil {
		log.Fatal(err)
	}
	install := flag.Bool("service-install", false, "install Portivane as an operating system service")
	uninstall := flag.Bool("service-uninstall", false, "remove the Portivane operating system service")
	flag.Parse()
	if *install {
		if err := program.Install(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *uninstall {
		if err := program.Uninstall(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := program.Run(); err != nil {
		log.Fatal(err)
	}
}

func runApp() {
	state, err := newStore()
	if err != nil {
		log.Fatal(err)
	}
	if services, discoverErr := discoverServices(); discoverErr == nil {
		state.mergeServices(services)
	} else {
		state.log(discoverErr.Error())
	}
	state.setAccount(inspectAccount())
	assets, err := fs.Sub(webFiles, "web/dist")

	// Auto-resume any tunnels that were live before Portivane last stopped.
	// They already have a tunnelId and hostname on disk — just re-run them.
	if state.state.Account.CloudflaredInstalled && state.state.Account.Connected {
		snap := state.snapshot()
		for _, svc := range snap.Services {
			if svc.TunnelID != "" && svc.Hostname != "" {
				go func(s Service) {
					state.serviceLog(s.ID, "Portivane restarted — resuming tunnel…")
					if resumeErr := startTunnel(state, s, s.Hostname); resumeErr != nil {
						state.serviceLog(s.ID, "Auto-resume failed: "+resumeErr.Error())
						state.updateService(s.ID, func(item *Service) { item.Status = "error" })
					}
				}(svc)
			}
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	app := &server{store: state, assets: assets}
	listenAddr := os.Getenv("PORTIVANE_LISTEN")
	if listenAddr == "" {
		listenAddr = "127.0.0.1:4747"
	}
	httpServer := &http.Server{Addr: listenAddr, Handler: app.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}

	go func() {
		if os.Getenv("PORTIVANE_NO_BROWSER") == "" && os.Getenv("WYRMHOLE_NO_BROWSER") == "" && os.Getenv("TUNNELWAY_NO_BROWSER") == "" && os.Getenv("PORTIVANE_SERVICE") != "1" {
			time.Sleep(250 * time.Millisecond)
			_ = openBrowser("http://127.0.0.1:4747")
		}
	}()
	go func() {
		log.Printf("Portivane is ready at http://127.0.0.1:4747")
		if serveErr := httpServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Fatal(serveErr)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	for id := range state.processes {
		_ = stopTunnel(state, id)
	}
	_ = httpServer.Close()
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
