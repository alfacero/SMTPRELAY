package main

import (
	"flag"
	"fmt"
	svc "github.com/kardianos/service"
	"os"
	"relay/config"
	"relay/logger"
	relayservice "relay/service"
)

func main() {
	iniPath := "relay.ini"
	run := flag.Bool("run", false, "modo consola")
	install := flag.Bool("install", false, "instalar servicio")
	uninstall := flag.Bool("uninstall", false, "desinstalar servicio")
	flag.StringVar(&iniPath, "config", "relay.ini", "ruta al INI")
	flag.Parse()

	cfg, err := config.Load(iniPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}

	if *run {
		os.Setenv("RELAY_CONSOLE", "1")
	}
	logger.Setup(cfg.Archivos.RutaLog, cfg.Log.Nivel)

	svcConfig := &svc.Config{
		Name:        "RelaySMTP",
		DisplayName: "Relay SMTP Local",
		Description: "Relay SMTP con cola persistente en SQLite",
	}
	prg := relayservice.Nuevo(cfg)
	s, err := svc.New(prg, svcConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *install {
		if err := s.Install(); err != nil {
			fmt.Fprintln(os.Stderr, "install:", err)
			os.Exit(1)
		}
		fmt.Println("servicio instalado. Ahora: net start RelaySMTP")
		return
	}
	if *uninstall {
		_ = s.Stop()
		if err := s.Uninstall(); err != nil {
			fmt.Fprintln(os.Stderr, "uninstall:", err)
			os.Exit(1)
		}
		fmt.Println("servicio desinstalado")
		return
	}

	if err := s.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
}
