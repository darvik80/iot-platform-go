package mail

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"

	"cogitor-iot-platform/internal/config"
)

type Sender struct {
	Cfg config.Config
	Log *slog.Logger
}

func (s *Sender) Send(to, subject, body string) error {
	from := s.Cfg.Email.SMTPFrom
	if from == "" {
		from = "noreply@cogitor.local"
	}
	msg := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	mode := s.Cfg.Email.Mode
	if mode == "" || (mode == "smtp" && s.Cfg.Email.SMTPHost == "") {
		mode = "log"
	}
	if mode == "log" {
		s.Log.Info("email (log mode)", "to", to, "subject", subject, "body", body)
		return nil
	}

	addr := fmt.Sprintf("%s:%d", s.Cfg.Email.SMTPHost, s.Cfg.Email.SMTPPort)
	var auth smtp.Auth
	if s.Cfg.Email.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.Cfg.Email.SMTPUser, s.Cfg.Email.SMTPPass, s.Cfg.Email.SMTPHost)
	}

	// Port 465 requires implicit SSL; other ports use STARTTLS via smtp.SendMail.
	if s.Cfg.Email.SMTPPort == 465 {
		return sendMailSSL(addr, from, auth, []string{to}, []byte(msg))
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

// sendMailSSL sends email via implicit SSL (port 465).
func sendMailSSL(addr, from string, auth smtp.Auth, to []string, msg []byte) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	tlsConfig := &tls.Config{ServerName: host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *Sender) SendVerification(to, login, verifyURL string) error {
	body := fmt.Sprintf(
		"Hello %s,\n\nConfirm your Cogitor IoT account by opening this link:\n%s\n\nIf you did not register, ignore this email.\n",
		login, verifyURL,
	)
	return s.Send(to, "Confirm your Cogitor IoT account", body)
}

func (s *Sender) SendInvitation(to, accountName, role, inviteURL string) error {
	body := fmt.Sprintf(
		"You have been invited to join %s as %s.\n\nAccept the invitation by opening this link:\n%s\n\nIf you did not expect this, ignore this email.\n",
		accountName, role, inviteURL,
	)
	return s.Send(to, fmt.Sprintf("Invitation to join %s", accountName), body)
}
