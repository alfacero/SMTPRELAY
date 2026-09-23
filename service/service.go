package service

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/kardianos/service"

	"relay/cola"
	"relay/config"
	"relay/smtpserver"
	"relay/webadmin"
	"relay/worker"
)

type Programa struct {
	cfg *config.Config
}

// Nuevo construye el Programa desde fuera del paquete.
func Nuevo(cfg *config.Config) *Programa {
	return &Programa{cfg: cfg}
}

func (p *Programa) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *Programa) run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	colaDB, err := cola.Abrir(p.cfg.Archivos.RutaDB)
	if err != nil {
		slog.Error("no se pudo abrir DB", "err", err)
		return
	}
	defer colaDB.Cerrar()

	if err := colaDB.RecuperarEnviando(); err != nil {
		slog.Warn("recuperar enviando", "err", err)
	}

	backend := &smtpserver.Backend{Cola: colaDB}
	srv, err := smtpserver.Iniciar(":"+strconv.Itoa(p.cfg.Servidor.PuertoLocal), backend)
	if err != nil {
		slog.Error("no se pudo iniciar SMTP", "err", err)
		return
	}
	defer srv.Close()

	webadmin.Iniciar(p.cfg, colaDB)

	w := worker.Nuevo(colaDB, p.cfg)
	w.Run(ctx)
}

func (p *Programa) Stop(s service.Service) error { return nil }
