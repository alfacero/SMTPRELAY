package webadmin

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"relay/cola"
	"relay/config"
)

type Admin struct {
	cfg  *config.Config
	cola *cola.ColaDB
}

func Iniciar(cfg *config.Config, colaDB *cola.ColaDB) {
	if !cfg.Web.Habilitado {
		return
	}

	a := &Admin{cfg: cfg, cola: colaDB}
	mux := http.NewServeMux()

	// Páginas HTML
	mux.HandleFunc("/", a.auth(a.handleDashboard))
	mux.HandleFunc("/correos", a.auth(a.handleListado))
	mux.HandleFunc("/correo/", a.auth(a.handleDetalle))
	mux.HandleFunc("/buscar", a.auth(a.handleBuscar))

	// Acciones (POST)
	mux.HandleFunc("/accion/forzar", a.auth(a.handleForzar))
	mux.HandleFunc("/accion/eliminar", a.auth(a.handleEliminar))
	mux.HandleFunc("/accion/reintentar-fallidos", a.auth(a.handleReintentarFallidos))

	// API JSON
	mux.HandleFunc("/api/stats", a.auth(a.handleAPIStats))
	mux.HandleFunc("/api/correos", a.auth(a.handleAPIListar))
	mux.HandleFunc("/api/correo/", a.auth(a.handleAPIDetalle))

	addr := cfg.Web.Bind + ":" + strconv.Itoa(cfg.Web.Puerto)
	go func() {
		slog.Info("webadmin escuchando", "addr", addr)
		srv := &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
		}
		if err := srv.ListenAndServe(); err != nil {
			slog.Error("webadmin murió", "err", err)
		}
	}()
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

func (a *Admin) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != a.cfg.Web.Usuario || p != a.cfg.Web.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Relay SMTP"`)
			http.Error(w, "no autorizado", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// ---------------------------------------------------------------------------
// Handlers HTML
// ---------------------------------------------------------------------------

func (a *Admin) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := a.cola.Estadisticas()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	ultimos, _ := a.cola.Listar(nil, 0, 10)

	render(w, tmplDashboard, map[string]any{
		"Stats":   stats,
		"Ultimos": ultimos,
		"Ahora":   time.Now(),
	})
}

func (a *Admin) handleListado(w http.ResponseWriter, r *http.Request) {
	estadoStr := r.URL.Query().Get("estado")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const limit = 50
	offset := (page - 1) * limit

	var estado *cola.Estado
	if estadoStr != "" {
		if e, err := strconv.Atoi(estadoStr); err == nil {
			est := cola.Estado(e)
			estado = &est
		}
	}

	correos, err := a.cola.Listar(estado, offset, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	render(w, tmplListado, map[string]any{
		"Correos":  correos,
		"Page":     page,
		"NextPage": page + 1,
		"PrevPage": page - 1,
		"Estado":   estadoStr,
		"Ahora":    time.Now(),
	})
}

func (a *Admin) handleDetalle(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/correo/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	co, err := a.cola.ObtenerPorID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, tmplDetalle, map[string]any{
		"Correo": co,
		"Raw":    string(co.RawMessage),
		"Ahora":  time.Now(),
	})
}

func (a *Admin) handleBuscar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Redirect(w, r, "/correos", http.StatusSeeOther)
		return
	}
	correos, err := a.cola.Buscar(q, 0, 100)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	render(w, tmplListado, map[string]any{
		"Correos":  correos,
		"Page":     1,
		"NextPage": 1,
		"PrevPage": 0,
		"Query":    q,
		"Ahora":    time.Now(),
	})
}

// ---------------------------------------------------------------------------
// Acciones (POST)
// ---------------------------------------------------------------------------

func (a *Admin) handleForzar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id inválido", 400)
		return
	}
	if err := a.cola.ForzarReintento(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	slog.Info("reintento forzado", "id", id)
	http.Redirect(w, r, "/correo/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *Admin) handleEliminar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id inválido", 400)
		return
	}
	if err := a.cola.Eliminar(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	slog.Info("correo eliminado", "id", id)
	http.Redirect(w, r, "/correos", http.StatusSeeOther)
}

func (a *Admin) handleReintentarFallidos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
		return
	}
	n, err := a.cola.ReintentarFallidos()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	slog.Info("fallidos reseteados", "cantidad", n)
	http.Redirect(w, r, "/correos?estado=0", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Handlers API JSON
// ---------------------------------------------------------------------------

func (a *Admin) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.cola.Estadisticas()
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonWrite(w, map[string]any{
		"pendientes":      stats.PorEstado[cola.Pendiente],
		"enviando":        stats.PorEstado[cola.Enviando],
		"enviados":        stats.PorEstado[cola.Enviado],
		"fallidos":        stats.PorEstado[cola.Fallido],
		"total":           stats.Total,
		"enviados_hoy":    stats.EnviadosHoy,
		"enviados_semana": stats.EnviadosSemana,
		"tasa_exito":      stats.TasaExito,
	})
}

func (a *Admin) handleAPIListar(w http.ResponseWriter, r *http.Request) {
	estadoStr := r.URL.Query().Get("estado")
	var estado *cola.Estado
	if estadoStr != "" {
		if e, err := strconv.Atoi(estadoStr); err == nil {
			est := cola.Estado(e)
			estado = &est
		}
	}
	limit := 100
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}
	correos, err := a.cola.Listar(estado, 0, limit)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonWrite(w, correos)
}

func (a *Admin) handleAPIDetalle(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/correo/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonError(w, err, 400)
		return
	}
	co, err := a.cola.ObtenerPorID(id)
	if err != nil {
		jsonError(w, err, 404)
		return
	}
	jsonWrite(w, co)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func render(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		slog.Error("template error", "err", err)
	}
}

func jsonWrite(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func jsonError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// EstadoNombre traduce el enum a texto para los templates.
func EstadoNombre(e cola.Estado) string {
	switch e {
	case cola.Pendiente:
		return "pendiente"
	case cola.Enviando:
		return "enviando"
	case cola.Enviado:
		return "enviado"
	case cola.Fallido:
		return "fallido"
	}
	return "desconocido"
}
