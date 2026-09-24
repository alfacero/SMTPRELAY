package service

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/kardianos/service"

	"relay/cola"
	"relay/config"
	"relay/smtpserver"
	"relay/webadmin"
	"relay/worker"
)

type Programa struct {
	cfg       *config.Config
	cancel    context.CancelFunc
	smtpSrv   *smtp.Server
	webSrv    *http.Server
	shutdownD chan struct{}
}

func Nuevo(cfg *config.Config) *Programa {
	return &Programa{
		cfg:       cfg,
		shutdownD: make(chan struct{}),
	}
}

func (p *Programa) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.run(ctx)
	return nil
}

func (p *Programa) run(ctx context.Context) {
	// Garantiza que shutdownD se cierre SIEMPRE, sin importar cómo salga run.
	defer close(p.shutdownD)

	// 1. Abrir Base de Datos
	colaDB, err := cola.Abrir(p.cfg.Archivos.RutaDB)
	if err != nil {
		slog.Error("no se pudo abrir DB", "err", err)
		return
	}
	defer colaDB.Cerrar()

	if err := colaDB.RecuperarEnviando(); err != nil {
		slog.Warn("recuperar enviando", "err", err)
	}

	// 2. Iniciar Servidor SMTP
	backend := &smtpserver.Backend{Cola: colaDB}
	p.smtpSrv, err = smtpserver.Iniciar(":"+strconv.Itoa(p.cfg.Servidor.PuertoLocal), backend)
	if err != nil {
		slog.Error("no se pudo iniciar SMTP", "err", err)
		return
	}
	defer p.smtpSrv.Close()

	// 3. Iniciar Panel Web Admin
	p.webSrv = webadmin.Iniciar(p.cfg, colaDB)
	if p.webSrv != nil {
		defer func() {
			ctxWeb, cancelWeb := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelWeb()
			_ = p.webSrv.Shutdown(ctxWeb)
		}()
	}

	// 4. Arrancar Worker (bloqueante hasta cancelación)
	w := worker.Nuevo(colaDB, p.cfg)
	w.Run(ctx)

	slog.Info("Todos los recursos locales del relay se han cerrado limpiamente.")
}

func (p *Programa) Stop(s service.Service) error {
	slog.Info("Windows ha solicitado detener el servicio. Iniciando apagado...")

	if p.cancel != nil {
		p.cancel()
	}

	select {
	case <-p.shutdownD:
		slog.Info("Apagado graceful completado con éxito.")
	case <-time.After(5 * time.Second):
		slog.Warn("El apagado tomó demasiado tiempo, forzando cierre de Windows.")
	}

	return nil
}
