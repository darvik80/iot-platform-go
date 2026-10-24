package mail

import (
	"strings"
	"testing"
)

func TestInvitationLooksLikeSiteAndLinkIsTheURL(t *testing.T) {
	u := "https://iot.cogitor.dev/invite/ZLUPZkqKTWuI1kn2GQSY7KfxCbdBhIJHBiNpvuJvLHg"
	text, html, err := invitationContent("Main", "viewer", u, "https://iot.cogitor.dev")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "\n"+u+"\n") {
		t.Fatalf("plain text must keep the URL on its own line:\n%s", text)
	}
	if strings.Contains(html, "Accept invitation") {
		t.Fatal("no button — the URL is the link")
	}
	wantAnchor := `<a href="` + u + `">` + u + `</a>`
	if !strings.Contains(html, wantAnchor) {
		t.Fatalf("missing bare URL link %q in:\n%s", wantAnchor, html)
	}
	if strings.Count(html, `href="`) != 1 {
		t.Fatalf("exactly one href, html:\n%s", html)
	}
	if strings.Contains(html, `href=""https`) || strings.Contains(html, `&quot;https`) {
		t.Fatal("extra quote around URL")
	}
	for _, want := range []string{
		"Cogitor",
		"#3d9cf0",
		"#0f1419",
		"#1c2430",
		`src="https://iot.cogitor.dev/favicon.png"`,
		"You have been invited to join Main as viewer",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
}

func TestVerificationHTMLHasURLLink(t *testing.T) {
	u := "https://iot.example/verify?token=abc"
	_, html, err := verificationContent("darvik", u, "https://iot.example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<a href="`+u+`">`+u+`</a>`) {
		t.Fatalf("html:\n%s", html)
	}
	if !strings.Contains(html, "Hello darvik,") {
		t.Fatal("missing greeting")
	}
}

func TestInvitationEscapesHTML(t *testing.T) {
	_, html, err := invitationContent(`Acme <script>alert(1)</script>`, `admin" onclick="x`, "https://iot.example/invite?token=1", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") || strings.Contains(html, `onclick="x`) {
		t.Fatalf("unescaped content in html: %s", html)
	}
	if !strings.Contains(html, "Acme &lt;script&gt;") {
		t.Fatalf("account name not escaped: %s", html)
	}
}

func TestBuildMessageHasBrandedHTMLAndPlainURL(t *testing.T) {
	u := "https://iot.cogitor.dev/invite/ZLUPZkqKTWuI1kn2GQSY7KfxCbdBhIJHBiNpvuJvLHg"
	text, html, err := invitationContent("Main", "viewer", u, "https://iot.cogitor.dev")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := buildMessage(`"Cogitor IoT" <noreply@example.com>`, "u@example.com", "Invitation to join Main", text, html)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(msg)
	for _, want := range []string{
		"MIME-Version: 1.0",
		"multipart/alternative",
		"text/html; charset=UTF-8",
		"text/plain; charset=UTF-8",
		`From: "Cogitor IoT" <noreply@example.com>`,
		`<a href="` + u + `">` + u + `</a>`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("message missing %q", want)
		}
	}
	if strings.Contains(raw, "multipart/related") || strings.Contains(raw, "cid:") {
		t.Fatal("CID images break links in Yandex/Mail.ru — use the site favicon URL instead")
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
