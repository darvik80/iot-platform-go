package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"cogitor-iot-platform/internal/config"
)

// No external SMTP hosts: all tests bind an ephemeral loopback port.
func alertSMTPListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func alertSMTPSender(t *testing.T, ln net.Listener) *Sender {
	t.Helper()
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := strconv.Atoi(port)
	return &Sender{Cfg: config.Config{Email: config.EmailConfig{SMTPHost: host, SMTPPort: p, SMTPFrom: "monitoring@example.test", Mode: "smtp"}}}
}

func TestSendAlertLocalSMTPAndEscapedPortalHTML(t *testing.T) {
	ln := alertSMTPListener(t)
	message := make(chan string, 1)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		text := textproto.NewConn(conn)
		if err := text.PrintfLine("220 local.test ESMTP"); err != nil {
			serverErr <- err
			return
		}
		for {
			line, err := text.ReadLine()
			if err != nil {
				serverErr <- err
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				err = text.PrintfLine("250 local.test")
			case strings.HasPrefix(line, "MAIL FROM:"):
				if line != "MAIL FROM:<monitoring@example.test>" {
					serverErr <- fmt.Errorf("wrong sender: %s", line)
					return
				}
				err = text.PrintfLine("250 OK")
			case strings.HasPrefix(line, "RCPT TO:"):
				if line != "RCPT TO:<operations@example.test>" {
					serverErr <- fmt.Errorf("wrong recipient: %s", line)
					return
				}
				err = text.PrintfLine("250 OK")
			case line == "DATA":
				if err = text.PrintfLine("354 Send message"); err != nil {
					serverErr <- err
					return
				}
				body, readErr := text.ReadDotBytes()
				if readErr != nil {
					serverErr <- readErr
					return
				}
				message <- string(body)
				err = text.PrintfLine("250 Accepted")
			case line == "QUIT":
				serverErr <- text.PrintfLine("221 Bye")
				return
			default:
				serverErr <- fmt.Errorf("unexpected command %q", line)
				return
			}
			if err != nil {
				serverErr <- err
				return
			}
		}
	}()
	sender := alertSMTPSender(t, ln)
	if err := sender.SendAlert(context.Background(), "operations@example.test", "<script>alert(1)</script>", "firing", "Unsafe <img src=x onerror=alert(1)> & value > 30"); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	body := <-message
	for _, want := range []string{"multipart/alternative", "text/plain", "text/html", "#0f1419", "#1c2430", "&lt;script&gt;", "&lt;img src=x onerror=alert(1)&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	htmlStart := strings.Index(body, "<!DOCTYPE html>")
	if htmlStart < 0 {
		t.Fatal("missing HTML")
	}
	html := body[htmlStart:]
	if strings.Contains(html, "<script>") || strings.Contains(html, "<img src=x") {
		t.Fatal("raw HTML injection")
	}
}

func TestCheckAlertSTARTTLS(t *testing.T) {
	for _, tt := range []struct {
		name           string
		peer           net.Addr
		allowPlaintext bool
	}{
		{"IPv4 loopback", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")}, true},
		{"IPv4 loopback range", &net.TCPAddr{IP: net.ParseIP("127.2.3.4")}, true},
		{"IPv6 loopback", &net.TCPAddr{IP: net.ParseIP("::1")}, true},
		{"mapped loopback", &net.TCPAddr{IP: net.ParseIP("::ffff:127.0.0.1")}, true},
		{"remote IPv4", &net.TCPAddr{IP: net.ParseIP("192.0.2.1")}, false},
		{"remote IPv6", &net.TCPAddr{IP: net.ParseIP("2001:db8::1")}, false},
		{"mapped remote", &net.TCPAddr{IP: net.ParseIP("::ffff:192.0.2.1")}, false},
		{"private IPv4", &net.TCPAddr{IP: net.ParseIP("10.0.0.1")}, false},
		{"private IPv6", &net.TCPAddr{IP: net.ParseIP("fd00::1")}, false},
		{"unspecified IPv4", &net.TCPAddr{IP: net.IPv4zero}, false},
		{"unspecified IPv6", &net.TCPAddr{IP: net.IPv6zero}, false},
		{"missing IP", &net.TCPAddr{}, false},
		{"nil TCP address", (*net.TCPAddr)(nil), false},
		{"unknown address", nil, false},
		{"hostname is not an IP", &net.UnixAddr{Name: "localhost", Net: "unix"}, false},
	} {
		for _, available := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/STARTTLS=%v", tt.name, available), func(t *testing.T) {
				err := checkAlertSTARTTLS(tt.peer, available)
				wantErr := !available && !tt.allowPlaintext
				if (err != nil) != wantErr {
					t.Fatalf("checkAlertSTARTTLS(%v, %v) = %v, want error: %v", tt.peer, available, err, wantErr)
				}
			})
		}
	}
}

func TestSendAlertSMTPDataResult(t *testing.T) {
	for _, tt := range []struct {
		name       string
		dataCode   int
		rejectQuit bool
	}{
		{"accepted then peer closes", 250, false},
		{"accepted then QUIT rejected", 250, true},
		{"DATA rejected", 550, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ln := alertSMTPListener(t)
			serve := func() error {
				conn, err := ln.Accept()
				if err != nil {
					return err
				}
				defer conn.Close()
				if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
					return err
				}
				text := textproto.NewConn(conn)
				if err := text.PrintfLine("220 local.test ESMTP"); err != nil {
					return err
				}
				for _, step := range []struct{ command, response string }{
					{"EHLO ", "250 local.test"},
					{"MAIL FROM:<from@example.test>", "250 OK"},
					{"RCPT TO:<to@example.test>", "250 OK"},
					{"DATA", "354 Send message"},
				} {
					line, err := text.ReadLine()
					if err != nil {
						return err
					}
					if !strings.HasPrefix(line, step.command) {
						return fmt.Errorf("expected %q, got %q", step.command, line)
					}
					if err := text.PrintfLine("%s", step.response); err != nil {
						return err
					}
				}
				body, err := text.ReadDotBytes()
				if err != nil {
					return err
				}
				if string(body) != "test\n" {
					return fmt.Errorf("unexpected DATA: %q", body)
				}
				if err := text.PrintfLine("%d Result", tt.dataCode); err != nil {
					return err
				}
				if tt.rejectQuit {
					line, err := text.ReadLine()
					if err != nil {
						return err
					}
					if line != "QUIT" {
						return fmt.Errorf("expected QUIT, got %q", line)
					}
					return text.PrintfLine("500 QUIT rejected")
				}
				return nil
			}
			serverErr := make(chan error, 1)
			go func() { serverErr <- serve() }()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := sendAlertSMTP(ctx, ln.Addr().String(), "from@example.test", nil, "to@example.test", []byte("test\r\n"), false)
			if tt.dataCode == 250 {
				if err != nil {
					t.Errorf("accepted message reported as failure: %v", err)
				}
			} else {
				var smtpErr *textproto.Error
				if !errors.As(err, &smtpErr) || smtpErr.Code != tt.dataCode {
					t.Errorf("expected DATA rejection %d, got %v", tt.dataCode, err)
				}
			}
			if err := <-serverErr; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSendAlertSMTPRejectsUnconfiguredAndHeaderInjection(t *testing.T) {
	sender := &Sender{}
	if err := sender.SendAlert(context.Background(), "to@example.test", "name", "firing", "message"); err == nil {
		t.Fatal("log mode falsely delivered alert")
	}
	ln := alertSMTPListener(t)
	sender = alertSMTPSender(t, ln)
	if err := sender.SendAlert(context.Background(), "to@example.test\r\nBcc:other@example.test", "name", "firing", "message"); err == nil {
		t.Fatal("recipient header injection accepted")
	}
	sender.Cfg.Email.SMTPFrom = "from@example.test\r\nBcc:other@example.test"
	if err := sender.SendAlert(context.Background(), "to@example.test", "name", "firing", "message"); err == nil {
		t.Fatal("sender header injection accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sender.SendAlert(ctx, "to@example.test", "name", "firing", "message"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSendAlertSMTPCancellationInterruptsGreeting(t *testing.T) {
	ln := alertSMTPListener(t)
	accepted := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = io.Copy(io.Discard, conn) // no SMTP greeting: caller must interrupt
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- alertSMTPSender(t, ln).SendAlert(ctx, "to@example.test", "name", "firing", "message") }()
	<-accepted
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel delivered message")
		}
	case <-time.After(time.Second):
		t.Fatal("SMTP greeting ignored cancellation")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("connection leaked")
	}
}

func TestSendAlertSMTPUsesSTARTTLSAndImplicitTLS(t *testing.T) {
	for _, implicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("implicit=%v", implicit), func(t *testing.T) {
			ln := alertSMTPListener(t)
			observed := make(chan error, 1)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					observed <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				reader := bufio.NewReader(conn)
				if !implicit {
					_, _ = io.WriteString(conn, "220 local.test ESMTP\r\n")
					line, err := reader.ReadString('\n')
					if err != nil || !strings.HasPrefix(line, "EHLO") {
						observed <- fmt.Errorf("missing EHLO: %q %v", line, err)
						return
					}
					_, _ = io.WriteString(conn, "250-local.test\r\n250 STARTTLS\r\n")
					line, err = reader.ReadString('\n')
					if err != nil || line != "STARTTLS\r\n" {
						observed <- fmt.Errorf("missing STARTTLS: %q %v", line, err)
						return
					}
					_, _ = io.WriteString(conn, "220 Begin TLS\r\n")
				}
				first, err := reader.ReadByte()
				if err == nil && first != 0x16 {
					err = fmt.Errorf("expected TLS handshake, got %x", first)
				}
				observed <- err // closing an intentionally incomplete TLS peer must fail the send
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := sendAlertSMTP(ctx, ln.Addr().String(), "from@example.test", nil, "to@example.test", []byte("test"), implicit); err == nil {
				t.Fatal("incomplete TLS delivered message")
			}
			if err := <-observed; err != nil {
				t.Fatal(err)
			}
		})
	}
}
