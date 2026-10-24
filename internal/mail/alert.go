package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SendAlert uses the configured SMTP sender, never log-mode delivery. All dynamic
// content remains plain strings in the portal's auto-escaping HTML template.
func (s *Sender) SendAlert(ctx context.Context, to, name, state, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Cfg.Email.EffectiveMode() != "smtp" || s.Cfg.Email.SMTPPort < 1 || s.Cfg.Email.SMTPPort > 65535 {
		return errors.New("alert SMTP is not configured")
	}
	if strings.ContainsAny(to+s.Cfg.Email.SMTPFrom, "\r\n") {
		return errors.New("invalid alert mail address")
	}
	recipient, err := netmail.ParseAddress(to)
	if err != nil || recipient.Address != to {
		return errors.New("invalid alert mail recipient")
	}
	from, err := netmail.ParseAddress(s.Cfg.Email.SMTPFrom)
	if err != nil {
		return errors.New("invalid alert mail sender")
	}
	html, err := renderHTML(emailContent{
		Title: "Monitoring alert", Lead: name, Badge: state, Body: message,
		FinePrint: "You received this notification from your Cogitor IoT monitoring rules.",
	})
	if err != nil {
		return err
	}
	msg, err := buildMessage(formatFrom(s.Cfg.Email.SMTPFrom), to, "[Cogitor IoT] "+state+": "+name,
		name+"\nState: "+state+"\n\n"+message+"\n", html)
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if s.Cfg.Email.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.Cfg.Email.SMTPUser, s.Cfg.Email.SMTPPass, s.Cfg.Email.SMTPHost)
	}
	addr := net.JoinHostPort(s.Cfg.Email.SMTPHost, strconv.Itoa(s.Cfg.Email.SMTPPort))
	return sendAlertSMTP(ctx, addr, from.Address, auth, recipient.Address, msg, s.Cfg.Email.SMTPPort == 465)
}

// Unlike smtp.SendMail, every phase (including greeting, DATA and QUIT) is
// bounded by the same deadline and interrupted by caller cancellation.
func sendAlertSMTP(ctx context.Context, addr, from string, auth smtp.Auth, to string, msg []byte, implicitTLS bool) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	var wire net.Conn = conn
	if implicitTLS {
		secured := tls.Client(conn, tlsConfig)
		if err := secured.HandshakeContext(ctx); err != nil {
			return err
		}
		wire = secured
	}
	client, err := smtp.NewClient(wire, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if !implicitTLS {
		available, _ := client.Extension("STARTTLS")
		if err := checkAlertSTARTTLS(conn.RemoteAddr(), available); err != nil {
			return err
		}
		if available {
			if err := client.StartTLS(tlsConfig); err != nil {
				return err
			}
		}
	}
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("SMTP authentication is unavailable")
		}
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	// DATA was accepted; a QUIT failure must not cause a duplicate delivery.
	_ = client.Quit()
	return nil
}

func checkAlertSTARTTLS(peer net.Addr, available bool) error {
	if available {
		return nil
	}
	tcp, ok := peer.(*net.TCPAddr)
	if ok && tcp != nil && tcp.IP.IsLoopback() {
		return nil
	}
	return errors.New("SMTP STARTTLS is required for non-loopback peers")
}
