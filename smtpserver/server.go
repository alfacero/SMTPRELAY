package smtpserver

import (
	"bytes"
	"github.com/emersion/go-smtp"
	"io"
	"log/slog"
	"relay/cola"
)

type Backend struct {
	Cola *cola.ColaDB
}

func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{cola: b.Cola}, nil
}

type Session struct {
	cola          *cola.ColaDB
	remitente     string
	destinatarios []string
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.remitente = from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.destinatarios = append(s.destinatarios, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	asunto := extraerAsunto(raw)

	id, err := s.cola.Encolar(s.remitente, joinAddrs(s.destinatarios), asunto, raw)
	if err != nil {
		slog.Error("error encolando", "err", err)
		return err
	}
	slog.Info("correo encolado", "id", id, "de", s.remitente, "para", s.destinatarios)
	return nil
}

func (s *Session) Reset()        { s.remitente = ""; s.destinatarios = nil }
func (s *Session) Logout() error { return nil }

func Iniciar(addr string, backend *Backend) (*smtp.Server, error) {
	s := smtp.NewServer(backend)
	s.Addr = addr
	s.Domain = "localhost"
	s.MaxMessageBytes = 25 * 1024 * 1024
	s.AllowInsecureAuth = true

	go func() {
		slog.Info("SMTP local escuchando", "addr", addr)
		if err := s.ListenAndServe(); err != nil {
			slog.Error("SMTP server murió", "err", err)
		}
	}()
	return s, nil
}

func extraerAsunto(raw []byte) string {
	// Simple: buscar "Subject: " hasta \r\n
	idx := bytes.Index(raw, []byte("Subject: "))
	if idx < 0 {
		return ""
	}
	resto := raw[idx+9:]
	if fin := bytes.Index(resto, []byte("\r\n")); fin >= 0 {
		return string(resto[:fin])
	}
	return ""
}

func joinAddrs(a []string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
