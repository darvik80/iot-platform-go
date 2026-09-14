package mail

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"cogitor-iot-platform/internal/config"
)

type Sender struct {
	Cfg config.Config
	Log *slog.Logger
}

func (s *Sender) Send(to, subject, body string) error {
	from := s.Cfg.SMTPFrom
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

	mode := s.Cfg.EmailMode
	if mode == "" || (mode == "smtp" && s.Cfg.SMTPHost == "") {
		mode = "log"
	}
	if mode == "log" {
		s.Log.Info("email (log mode)", "to", to, "subject", subject, "body", body)
		return nil
	}

	addr := fmt.Sprintf("%s:%d", s.Cfg.SMTPHost, s.Cfg.SMTPPort)
	var auth smtp.Auth
	if s.Cfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.Cfg.SMTPUser, s.Cfg.SMTPPass, s.Cfg.SMTPHost)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
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
