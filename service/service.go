package service

import (
	"context"
	smtp "github.com/emersion/go-smtp"
	service "github.com/kardianos/service"
	"log/slog"
	"net/http"
	"relay/cola"
	"relay/config"
	"relay/smtpserver"
	"relay/webadmin"
	"relay/worker"
	"strconv"
	"time"
)

type Programa struct {
	cfg       *config.Config
	cancel    context.CancelFunc
	smtpSrv   *smtp.Server
	webSrv    *http.Server
	shutdownD chan struct{} // Canal para notificar a Windows que terminamos de limpiar
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
			// Cierre suave de conexiones HTTP (le damos 2s max al apagarse)
			ctxWeb, cancelWeb := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancelWeb()
			_ = p.webSrv.Shutdown(ctxWeb)
		}()
	}

	// 4. Arrancar Worker (Bloqueante síncrono)
	w := worker.Nuevo(colaDB, p.cfg)
	w.Run(ctx)

	// Al salir de w.Run por la cancelación, avanzará hacia acá ejecutando los defers.
	slog.Info("Todos los recursos locales del relay se han cerrado limpiamente.")
	close(p.shutdownD) // Avisamos a Stop() que ya terminó la limpieza
}

func (p *Programa) Stop(s service.Service) error {
	slog.Info("Windows ha solicitado detener el servicio. Iniciando apagado...")

	if p.cancel != nil {
		p.cancel() // Cancela el contexto, liberando al worker y rompiendo el ciclo.
	}

	// Esperamos a que la goroutine de run() termine de ejecutar los defers
	// o forzamos la salida si toma más de 5 segundos.
	select {
	case <-p.shutdownD:
		slog.Info("Apagado graceful completado con éxito.")
	case <-time.After(5 * time.Second):
		slog.Warn("El apagado tomó demasiado tiempo, forzando cierre de Windows.")
	}

	return nil
}
