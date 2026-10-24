package mail

import (
	"strings"
	"testing"
)

func TestInvitationPlaintextContainsURL(t *testing.T) {
	s := &Sender{}
	u := "https://iot.cogitor.dev/invite/ZLUPZkqKTWuI1kn2GQSY7KfxCbdBhIJHBiNpvuJvLHg"
	// log mode: no SMTP, just build the same body SendInvitation would send
	body := invitationBody("Main", "viewer", u)
	if !strings.Contains(body, "You have been invited to join Main as viewer.") {
		t.Fatalf("body: %s", body)
	}
	if strings.Contains(body, "Accept invitation") {
		t.Fatal("no button label — the URL is the link")
	}
	if !strings.Contains(body, "\n"+u+"\n") {
		t.Fatalf("URL must be on its own line so mail clients autolink it:\n%s", body)
	}
	_ = s
}

func TestVerificationPlaintextContainsURL(t *testing.T) {
	u := "https://iot.example/verify?token=abc"
	body := verificationBody("darvik", u)
	if !strings.Contains(body, "Hello darvik,") {
		t.Fatalf("body: %s", body)
	}
	if !strings.Contains(body, "\n"+u+"\n") {
		t.Fatalf("URL must be on its own line:\n%s", body)
	}
}

func TestBuildMessagePlaintextOnly(t *testing.T) {
	u := "https://iot.cogitor.dev/invite/ZLUPZkqKTWuI1kn2GQSY7KfxCbdBhIJHBiNpvuJvLHg"
	body := invitationBody("Main", "viewer", u)
	raw := string(buildMessage(`"Cogitor IoT" <noreply@example.com>`, "u@example.com", "Invitation to join Main", body))
	for _, want := range []string{
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		`From: "Cogitor IoT" <noreply@example.com>`,
		u,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("message missing %q", want)
		}
	}
	if strings.Contains(raw, "text/html") || strings.Contains(raw, "multipart/") || strings.Contains(raw, "<a href") {
		t.Fatal("must be plaintext only — HTML breaks autolink in Yandex/Mail.ru")
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
