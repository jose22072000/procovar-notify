package channels

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP es un servidor SMTP mínimo (sin TLS ni auth) para probar el envío sin
// red externa: responde lo justo a EHLO/MAIL/RCPT/DATA/QUIT y captura el sobre.
type fakeSMTP struct {
	ln       net.Listener
	mu       sync.Mutex
	mailFrom string
	rcptTo   []string
	data     string
	gotData  bool
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTP{ln: ln}
	go s.accept()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *fakeSMTP) addr() (host string, port int) {
	a := s.ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", a.Port
}

func (s *fakeSMTP) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSMTP) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(conn)
	w := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }

	w("220 fake ESMTP")
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			w("250-fake greets you")
			w("250-8BITMIME")
			w("250-SMTPUTF8")
			w("250 SIZE 52428800")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			s.mu.Lock()
			s.mailFrom = strings.TrimSpace(line[len("MAIL FROM"):])
			s.mu.Unlock()
			w("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO"):
			s.mu.Lock()
			s.rcptTo = append(s.rcptTo, strings.TrimSpace(line[len("RCPT TO"):]))
			s.mu.Unlock()
			w("250 OK")
		case strings.HasPrefix(cmd, "DATA"):
			w("354 End data with <CR><LF>.<CR><LF>")
			var sb strings.Builder
			for {
				dl, err := br.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dl, "\r\n") == "." {
					break
				}
				sb.WriteString(dl)
			}
			s.mu.Lock()
			s.data = sb.String()
			s.gotData = true
			s.mu.Unlock()
			w("250 OK queued")
		case strings.HasPrefix(cmd, "QUIT"):
			w("221 Bye")
			return
		case strings.HasPrefix(cmd, "RSET"), strings.HasPrefix(cmd, "NOOP"):
			w("250 OK")
		default:
			w("250 OK")
		}
	}
}

func TestEmailSender_SendsOverSMTP(t *testing.T) {
	srv := newFakeSMTP(t)
	host, port := srv.addr()

	msg := RenderedMessage{
		Subject:   "Hola",
		HTMLBody:  "<p>Bienvenida {{x}}</p>",
		Recipient: Recipient{Email: "user@acme.test", Name: "Jane"},
	}
	route := ResolvedRoute{
		Channel: "EMAIL",
		SMTP: &SMTPConfig{
			Host: host, Port: port, FromEmail: "noreply@acme.test", FromName: "Acme", Secure: false,
		},
	}

	ref, err := NewEmailSender().Send(context.Background(), msg, route)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.HasSuffix(string(ref), "@qb-notify") {
		t.Fatalf("ProviderRef debería ser el Message-ID, got %q", ref)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !srv.gotData {
		t.Fatal("el servidor no recibió DATA")
	}
	if !strings.Contains(srv.mailFrom, "noreply@acme.test") {
		t.Errorf("MAIL FROM inesperado: %q", srv.mailFrom)
	}
	if len(srv.rcptTo) != 1 || !strings.Contains(srv.rcptTo[0], "user@acme.test") {
		t.Errorf("RCPT TO inesperado: %v", srv.rcptTo)
	}
	if !strings.Contains(srv.data, "Subject: Hola") {
		t.Errorf("el cuerpo debería incluir el asunto; got:\n%s", srv.data)
	}
}

func TestEmailSender_Validation(t *testing.T) {
	s := NewEmailSender()
	if s.Channel() != "EMAIL" {
		t.Fatalf("canal = %q, want EMAIL", s.Channel())
	}

	// Sin conexión SMTP en la ruta.
	if _, err := s.Send(context.Background(), RenderedMessage{Recipient: Recipient{Email: "a@b.c"}}, ResolvedRoute{Channel: "EMAIL"}); err == nil {
		t.Error("sin SMTP debería dar error")
	}
	// Sin email de destinatario.
	route := ResolvedRoute{Channel: "EMAIL", SMTP: &SMTPConfig{Host: "127.0.0.1", Port: 25, FromEmail: "n@a.test", FromName: "A"}}
	if _, err := s.Send(context.Background(), RenderedMessage{}, route); err == nil {
		t.Error("sin email de destinatario debería dar error")
	}
}
