package mail

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
	"unicode"

	"cogitor-iot-platform/internal/config"
)

type Sender struct {
	Cfg config.Config
	Log *slog.Logger
}

func (s *Sender) Send(to, subject, body string) error {
	fromAddr := s.Cfg.Email.SMTPFrom
	if fromAddr == "" {
		fromAddr = "noreply@cogitor.local"
	}
	from := formatFrom(fromAddr)

	mode := s.Cfg.Email.EffectiveMode()
	if mode == "log" {
		if s.Log != nil {
			s.Log.Info("email (log mode)", "to", to, "subject", subject, "body", body)
		}
		return nil
	}

	msg := buildMessage(from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", s.Cfg.Email.SMTPHost, s.Cfg.Email.SMTPPort)
	var auth smtp.Auth
	if s.Cfg.Email.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.Cfg.Email.SMTPUser, s.Cfg.Email.SMTPPass, s.Cfg.Email.SMTPHost)
	}

	var err error
	if s.Cfg.Email.SMTPPort == 465 {
		err = sendMailSSL(addr, fromAddr, auth, []string{to}, msg)
	} else {
		err = smtp.SendMail(addr, auth, fromAddr, []string{to}, msg)
	}
	if err != nil {
		s.logSendErr(to, subject, err)
		return err
	}
	if s.Log != nil {
		s.Log.Info("email sent", "to", to, "subject", subject, "host", s.Cfg.Email.SMTPHost, "port", s.Cfg.Email.SMTPPort)
	}
	return nil
}

func (s *Sender) SendVerification(to, login, verifyURL string) error {
	return s.Send(to, "Confirm your Cogitor IoT account", verificationBody(login, verifyURL))
}

func (s *Sender) SendInvitation(to, accountName, role, inviteURL string) error {
	accountName = strings.TrimSpace(accountName)
	if accountName == "" {
		accountName = "a Cogitor IoT account"
	}
	return s.Send(to, fmt.Sprintf("Invitation to join %s", accountName), invitationBody(accountName, role, inviteURL))
}

func verificationBody(login, verifyURL string) string {
	login = strings.TrimSpace(login)
	hello := "Hello,"
	if login != "" {
		hello = "Hello " + login + ","
	}
	return hello + "\n\nConfirm your Cogitor IoT account by opening this link:\n" + strings.TrimSpace(verifyURL) + "\n\nIf you did not register, ignore this email.\n"
}

func invitationBody(accountName, role, inviteURL string) string {
	accountName = strings.TrimSpace(accountName)
	role = strings.TrimSpace(role)
	if accountName == "" {
		accountName = "a Cogitor IoT account"
	}
	body := "You have been invited to join " + accountName
	if role != "" {
		body += " as " + role
	}
	return body + ".\n\nAccept the invitation by opening this link:\n" + strings.TrimSpace(inviteURL) + "\n\nIf you did not expect this, ignore this email.\n"
}

func (s *Sender) logSendErr(to, subject string, err error) {
	if s.Log == nil {
		return
	}
	s.Log.Error("email send failed", "to", to, "subject", subject, "host", s.Cfg.Email.SMTPHost, "port", s.Cfg.Email.SMTPPort, "err", err)
}

func formatFrom(addr string) string {
	if strings.Contains(addr, "<") {
		return addr
	}
	return `"Cogitor IoT" <` + addr + `>`
}

func encodeHeader(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", "")
	for _, r := range s {
		if r > unicode.MaxASCII {
			return mime.QEncoding.Encode("UTF-8", s)
		}
	}
	return s
}

func buildMessage(from, to, subject, body string) []byte {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", encodeHeader(subject))
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	msg.WriteString(body)
	return msg.Bytes()
}

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
