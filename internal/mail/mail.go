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
	return s.sendMIME(to, subject, body, "")
}

func (s *Sender) SendVerification(to, login, verifyURL string) error {
	text, html, err := verificationContent(login, verifyURL, s.Cfg.Platform.PublicBaseURL)
	if err != nil {
		return err
	}
	return s.sendMIME(to, "Confirm your Cogitor IoT account", text, html)
}

func (s *Sender) SendInvitation(to, accountName, role, inviteURL string) error {
	accountName = strings.TrimSpace(accountName)
	if accountName == "" {
		accountName = "a Cogitor IoT account"
	}
	text, html, err := invitationContent(accountName, role, inviteURL, s.Cfg.Platform.PublicBaseURL)
	if err != nil {
		return err
	}
	return s.sendMIME(to, fmt.Sprintf("Invitation to join %s", accountName), text, html)
}

func (s *Sender) sendMIME(to, subject, text, html string) error {
	fromAddr := s.Cfg.Email.SMTPFrom
	if fromAddr == "" {
		fromAddr = "noreply@cogitor.local"
	}
	from := formatFrom(fromAddr)

	mode := s.Cfg.Email.EffectiveMode()
	if mode == "log" {
		if s.Log != nil {
			s.Log.Info("email (log mode)", "to", to, "subject", subject, "body", text)
		}
		return nil
	}

	msg, err := buildMessage(from, to, subject, text, html)
	if err != nil {
		s.logSendErr(to, subject, err)
		return err
	}

	addr := fmt.Sprintf("%s:%d", s.Cfg.Email.SMTPHost, s.Cfg.Email.SMTPPort)
	var auth smtp.Auth
	if s.Cfg.Email.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.Cfg.Email.SMTPUser, s.Cfg.Email.SMTPPass, s.Cfg.Email.SMTPHost)
	}

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

func buildMessage(from, to, subject, text, html string) ([]byte, error) {
	var body bytes.Buffer
	alt := multipart.NewWriter(&body)
	if err := writePart(alt, "text/plain; charset=UTF-8", text); err != nil {
		return nil, err
	}
	if html != "" {
		if err := writePart(alt, "text/html; charset=UTF-8", html); err != nil {
			return nil, err
		}
	}
	if err := alt.Close(); err != nil {
		return nil, err
	}

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", encodeHeader(subject))
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	msg.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", alt.Boundary())
	msg.Write(body.Bytes())
	return msg.Bytes(), nil
}

func writePart(w *multipart.Writer, contentType, body string) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", contentType)
	h.Set("Content-Transfer-Encoding", "8bit")
	p, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	_, err = p.Write([]byte(body))
	return err
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
