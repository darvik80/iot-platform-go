package mail

import (
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
		"Cogitor",
		"#3d9cf0",
		"Confirm account",
		"Hello darvik",
		`href="https://iot.example/verify?token=abc"`,
		">Confirm account</a>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
}

func TestInvitationButtonHrefMatchesVisibleURL(t *testing.T) {
	u := "https://iot.example/invite/abcdefghijklmnopqrstuvwxyz0123456789-_"
	_, html, err := invitationContent("Acme", "viewer", u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<a href="`+u+`"`) || !strings.Contains(html, `>Accept invitation</a>`) {
		t.Fatalf("Accept invitation must be a link to the invite URL, html:\n%s", html)
	}
	if strings.Count(html, `href="`+u+`"`) != 1 {
		t.Fatalf("href should appear once, html:\n%s", html)
	}
	if !strings.Contains(html, `https:<span></span>//iot.example/invite/`) {
		t.Fatalf("copyable URL missing, html:\n%s", html)
	}
}

func TestSafeHTTPURL(t *testing.T) {
	if got := safeHTTPURL("iot.example/invite/tok"); got != "https://iot.example/invite/tok" {
		t.Fatalf("got %q", got)
	}
	if got := safeHTTPURL("javascript:alert(1)"); got != "" {
		t.Fatalf("unsafe url %q", got)
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

func TestBuildMessageAlternativeOnly(t *testing.T) {
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
		"multipart/alternative",
		"text/html; charset=UTF-8",
		"text/plain; charset=UTF-8",
		`From: "Cogitor IoT" <noreply@example.com>`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("message missing %q", want)
		}
	}
	if strings.Contains(raw, "multipart/related") || strings.Contains(raw, "cid:") {
		t.Fatal("inline CID images break links in Yandex/Mail.ru")
	}
	if !strings.Contains(raw, "8bit") {
		t.Fatal("html part should be 8bit so href is not encoded/wrapped")
	}
	if !strings.Contains(raw, `<a href="https://iot.example/v"`) {
		t.Fatalf("href must appear as 8bit plaintext, not encoded:\n%s", raw)
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
