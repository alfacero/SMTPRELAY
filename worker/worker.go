package worker

import (
    "context"
    "crypto/tls"
    "fmt"
    "log/slog"
    "net"
    "net/smtp"
    "strings"
    "time"

    "relay/cola"
    "relay/config"
)

type Worker struct {
    cola   *cola.ColaDB
    cfg    *config.Config
    ticker *time.Ticker
}

func Nuevo(c *cola.ColaDB, cfg *config.Config) *Worker {
    return &Worker{
        cola:   c,
        cfg:    cfg,
        ticker: time.NewTicker(time.Duration(cfg.Cola.IntervaloWorkerMs) * time.Millisecond),
    }
}

func (w *Worker) Run(ctx context.Context) {
    slog.Info("worker iniciado", "intervalo_ms", w.cfg.Cola.IntervaloWorkerMs)
    for {
        select {
        case <-ctx.Done():
            slog.Info("worker detenido")
            return
        case <-w.ticker.C:
            w.procesarUno(ctx)
        }
    }
}

func (w *Worker) procesarUno(ctx context.Context) {
    co, err := w.cola.TomarSiguientePendiente()
    if err != nil {
        slog.Error("error tomando pendiente", "err", err)
        return
    }
    if co == nil {
        return
    }

    err = w.enviar(ctx, co)
    if err == nil {
        if err := w.cola.MarcarEnviado(co.ID); err != nil {
            slog.Error("no se pudo marcar enviado", "id", co.ID, "err", err)
            return
        }
        slog.Info("correo enviado", "id", co.ID, "para", co.Destinatarios)
        return
    }

    intentos := co.Intentos + 1
    errMsg := err.Error()
    slog.Warn("fallo envío", "id", co.ID, "intentos", intentos, "err", errMsg)

    if intentos >= w.cfg.Cola.MaxIntentos ||
        time.Since(co.Creado) > time.Duration(w.cfg.Cola.DiasMaxReintento)*24*time.Hour {
        slog.Error("correo descartado por límite", "id", co.ID)
        _ = w.cola.MarcarFallidoDefinitivo(co.ID, errMsg)
        return
    }

    prox := time.Now().Add(backoff(intentos))
    if err := w.cola.RegistrarFallo(co.ID, intentos, errMsg, prox); err != nil {
        slog.Error("no se pudo registrar fallo", "id", co.ID, "err", err)
    } else {
        slog.Info("reintento programado", "id", co.ID, "prox", prox, "espera", backoff(intentos))
    }
}

func backoff(intentos int) time.Duration {
    switch intentos {
    case 1:
        return 1 * time.Minute
    case 2:
        return 5 * time.Minute
    case 3:
        return 15 * time.Minute
    case 4:
        return 1 * time.Hour
    case 5:
        return 4 * time.Hour
    case 6:
        return 12 * time.Hour
    case 7:
        return 24 * time.Hour
    default:
        return 48 * time.Hour
    }
}

// ---------------------------------------------------------------------------
// ENVÍO TRANSPARENTE: no parsea, no reconstruye, no toca un byte del mensaje.
// ---------------------------------------------------------------------------
func (w *Worker) enviar(ctx context.Context, co *cola.Correo) error {
    cfg := w.cfg.Destino

    // 1. Resolver destinatarios del envelope (los guardamos separados por coma)
    rcpts := parseDestinatarios(co.Destinatarios)
    if len(rcpts) == 0 {
        return fmt.Errorf("sin destinatarios en el envelope")
    }

    // 2. Conexión TCP (con timeout y respetando ctx)
    dialer := &net.Dialer{
        Timeout:   15 * time.Second,
        KeepAlive: 30 * time.Second,
    }
    hostPort := net.JoinHostPort(cfg.Servidor, itoa(cfg.Puerto))
    conn, err := dialer.DialContext(ctx, "tcp", hostPort)
    if err != nil {
        return fmt.Errorf("dial %s: %w", hostPort, err)
    }
    defer conn.Close()

    // Deadline global del envío (evita que un servidor colgado bloquee el worker)
    _ = conn.SetDeadline(time.Now().Add(60 * time.Second))

    // 3. Handshake inicial del servidor (banner 220)
    //    net/smtp.NewClient lo lee automáticamente, pero para STARTTLS a mano
    //    usamos smtp.NewClient y dejamos que él negocie.
    client, err := smtp.NewClient(conn, cfg.Servidor)
    if err != nil {
        return fmt.Errorf("smtp handshake: %w", err)
    }
    defer client.Close()

    // 4. STARTTLS o SSL implícito
    if cfg.UsarTLS {
        switch strings.ToLower(cfg.TipoTLS) {
        case "implicit":
            // Para 465 el TLS se negocia ANTES del banner SMTP.
            // net/smtp no lo soporta directo: envolvemos el conn en tls.Client
            // y ahí sí NewClient. Esto se maneja arriba, ver nota al pie.
            return fmt.Errorf("TLS implícito (465) no soportado con net/smtp; ver nota")
        default: // explicit (587 STARTTLS)
            tlsCfg := &tls.Config{
                ServerName: cfg.Servidor,
                MinVersion: tls.VersionTLS12,
            }
            if err := client.StartTLS(tlsCfg); err != nil {
                return fmt.Errorf("STARTTLS: %w", err)
            }
        }
    }

    // 5. Autenticación
    if cfg.UsarAuth {
        auth := smtp.PlainAuth("", cfg.Usuario, cfg.Password, cfg.Servidor)
        if err := client.Auth(auth); err != nil {
            return fmt.Errorf("auth: %w", err)
        }
    }

    // 6. Envelope: MAIL FROM + RCPT TO (del envelope original, NO de las cabeceras)
    if err := client.Mail(co.Remitente); err != nil {
        return fmt.Errorf("MAIL FROM <%s>: %w", co.Remitente, err)
    }
    for _, rcpt := range rcpts {
        if err := client.Rcpt(rcpt); err != nil {
            return fmt.Errorf("RCPT TO <%s>: %w", rcpt, err)
        }
    }

    // 7. DATA: escribimos el raw_message TAL CUAL.
    wc, err := client.Data()
    if err != nil {
        return fmt.Errorf("DATA: %w", err)
    }

    // Normalizar terminadores de línea: SMTP exige CRLF.
    // El mensaje guardado puede tener LF sueltos (según cómo entró).
    // Esta es la ÚNICA transformación que hacemos, y es inocua para DKIM
    // porque la canonicalización relaxed de DKIM ya colapsa CRLF/LF.
    raw := normalizarCRLF(co.RawMessage)

    if _, err := wc.Write(raw); err != nil {
        wc.Close() // best effort
        return fmt.Errorf("write body: %w", err)
    }
    if err := wc.Close(); err != nil {
        return fmt.Errorf("close DATA: %w", err)
    }

    // 8. QUIT limpio
    if err := client.Quit(); err != nil {
        // No es fatal: el mensaje ya fue aceptado con 250.
        slog.Warn("QUIT falló (mensaje ya aceptado)", "err", err)
    }
    return nil
}

// parseDestinatarios: "a@x.com,b@y.com" → ["a@x.com", "b@y.com"]
func parseDestinatarios(s string) []string {
    var out []string
    for _, p := range strings.Split(s, ",") {
        p = strings.TrimSpace(p)
        if p != "" {
            out = append(out, p)
        }
    }
    return out
}

// normalizarCRLF convierte LF solos a CRLF, respetando los CRLF ya presentes.
// SMTP (RFC 5321) exige CRLF como terminador de línea en DATA.
func normalizarCRLF(b []byte) []byte {
    // Fast path: si ya tiene CRLF en todos lados, devolvemos tal cual.
    // Igual hacemos una pasada; es O(n) y barato.
    out := make([]byte, 0, len(b)+len(b)/16)
    for i := 0; i < len(b); i++ {
        c := b[i]
        if c == '\n' && (i == 0 || b[i-1] != '\r') {
            out = append(out, '\r', '\n')
        } else {
            out = append(out, c)
        }
    }
    return out
}

func itoa(i int) string {
    // evitamos importar strconv solo para esto
    if i == 0 {
        return "0"
    }
    var buf [20]byte
    pos := len(buf)
    neg := i < 0
    if neg {
        i = -i
    }
    for i > 0 {
        pos--
        buf[pos] = byte('0' + i%10)
        i /= 10
    }
    if neg {
        pos--
        buf[pos] = '-'
    }
    return string(buf[pos:])
}