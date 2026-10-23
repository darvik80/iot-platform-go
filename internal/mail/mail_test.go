package mail

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestVerificationHTMLMatchesBrand(t *testing.T) {
	text, html, err := verificationContent("darvik", "https://iot.example/verify?token=abc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "https://iot.example/verify?token=abc") {
		t.Fatalf("plain text missing link: %s", text)
	}
	for _, want := range []string{
		"cid:cogitor-logo",
		"Cogitor",
		"#3d9cf0",
		"#0f1419",
		"#1c2430",
		"Confirm account",
		"Hello darvik",
		"https://iot.example/verify?token=abc",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
}

func TestInvitationEscapesHTML(t *testing.T) {
	_, html, err := invitationContent(`Acme <script>alert(1)</script>`, `admin" onclick="x`, "https://iot.example/invite?token=1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") || strings.Contains(html, `onclick="x`) {
		t.Fatalf("unescaped content in html: %s", html)
	}
	if !strings.Contains(html, "Acme &lt;script&gt;") {
		t.Fatalf("account name not escaped: %s", html)
	}
	if !strings.Contains(html, "Accept invitation") {
		t.Fatal("missing button")
	}
}

func TestBuildMessageEmbedsLogo(t *testing.T) {
	if len(logoPNG) < 100 || string(logoPNG[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("logo.png is not a PNG (%d bytes)", len(logoPNG))
	}
	text, html, err := verificationContent("u", "https://iot.example/v")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := buildMessage(`"Cogitor IoT" <noreply@example.com>`, "u@example.com", "Confirm your Cogitor IoT account", text, html)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(msg)
	for _, want := range []string{
		"MIME-Version: 1.0",
		"multipart/related",
		"multipart/alternative",
		"text/html; charset=UTF-8",
		"text/plain; charset=UTF-8",
		"image/png",
		"<cogitor-logo>",
		`From: "Cogitor IoT" <noreply@example.com>`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("message missing %q", want)
		}
	}
	encoded := base64.StdEncoding.EncodeToString(logoPNG)
	// base64 may be wrapped; compare without whitespace
	compact := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' {
			return -1
		}
		return r
	}, raw)
	if !strings.Contains(compact, encoded) {
		t.Fatal("message does not contain logo payload")
	}
	if !bytes.Contains(msg, []byte("cid:cogitor-logo")) {
		t.Fatal("html does not reference cid")
	}
}

func TestEncodeHeaderUTF8(t *testing.T) {
	got := encodeHeader("Invitation to join Когитор")
	if !strings.Contains(got, "=?UTF-8?") {
		t.Fatalf("expected RFC 2047 encoding, got %q", got)
	}
}

func TestFormatFrom(t *testing.T) {
	if got := formatFrom("noreply@cogitor.local"); got != `"Cogitor IoT" <noreply@cogitor.local>` {
		t.Fatalf("got %q", got)
	}
	if got := formatFrom(`Ops <ops@acme.test>`); got != `Ops <ops@acme.test>` {
		t.Fatalf("kept existing name: %q", got)
	}
}
