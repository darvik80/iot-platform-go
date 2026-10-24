package mail

import (
	"bytes"
	"html"
	"html/template"
	"net/url"
	"strings"
)

type emailContent struct {
	Title     string
	Lead      string
	Body      string
	Badge     string
	LogoURL   template.URL
	LinkHTML  template.HTML
	FinePrint string
}

func logoSrc(publicBase string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return ""
	}
	return base + "/favicon.png"
}

// linkHTML is a bare <a href="url">url</a>. No button styles: mail clients
// drop those and take the href with them. Color lives on the wrapping <p>.
func linkHTML(raw string) template.HTML {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	esc := html.EscapeString(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return template.HTML(esc)
	}
	return template.HTML(`<a href="` + esc + `">` + esc + `</a>`)
}

var emailTmpl = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
</head>
<body style="margin:0;padding:0;background-color:#0f1419;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#0f1419;">
  <tr>
    <td align="center" style="padding:32px 16px;">
      <table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:560px;">
        <tr>
          <td style="padding:0 0 20px 0;font-family:Arial,Helvetica,sans-serif;font-size:18px;font-weight:bold;color:#e6edf3;">
            {{if .LogoURL}}<img src="{{.LogoURL}}" width="22" height="22" alt="" style="width:22px;height:22px;border:0;vertical-align:middle;margin-right:8px;border-radius:4px;">{{end}}Cogitor <span style="color:#3d9cf0;">IoT</span>
          </td>
        </tr>
        <tr>
          <td style="background-color:#1c2430;border:1px solid #2a3544;padding:28px 24px;">
            <h1 style="margin:0 0 12px 0;font-family:Arial,Helvetica,sans-serif;font-size:22px;color:#e6edf3;">{{.Title}}</h1>
            {{if .Lead}}<p style="margin:0 0 16px 0;font-family:Arial,Helvetica,sans-serif;font-size:15px;line-height:1.5;color:#e6edf3;">{{.Lead}}{{if .Badge}} as {{.Badge}}{{end}}</p>{{end}}
            {{if .Body}}<p style="margin:0 0 16px 0;font-family:Arial,Helvetica,sans-serif;font-size:15px;line-height:1.5;color:#8b9bb0;">{{.Body}}</p>{{end}}
            {{if .LinkHTML}}<p style="margin:0;font-family:Arial,Helvetica,sans-serif;font-size:15px;line-height:1.5;color:#3d9cf0;word-break:break-all;">{{.LinkHTML}}</p>{{end}}
          </td>
        </tr>
        <tr>
          <td style="padding:20px 0 0 0;font-family:Arial,Helvetica,sans-serif;font-size:12px;color:#8b9bb0;">{{.FinePrint}}</td>
        </tr>
      </table>
    </td>
  </tr>
</table>
</body>
</html>`))

func renderHTML(c emailContent) (string, error) {
	var buf bytes.Buffer
	if err := emailTmpl.Execute(&buf, c); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func verificationContent(login, verifyURL, publicBase string) (text string, htmlOut string, err error) {
	login = strings.TrimSpace(login)
	hello := "Hello,"
	if login != "" {
		hello = "Hello " + login + ","
	}
	text = verificationBody(login, verifyURL)
	htmlOut, err = renderHTML(emailContent{
		Title:     "Confirm your account",
		Lead:      hello,
		Body:      "Open this link to confirm this email for your Cogitor IoT account.",
		LogoURL:   template.URL(logoSrc(publicBase)),
		LinkHTML:  linkHTML(verifyURL),
		FinePrint: "If you did not register, you can ignore this email.",
	})
	return text, htmlOut, err
}

func invitationContent(accountName, role, inviteURL, publicBase string) (text string, htmlOut string, err error) {
	text = invitationBody(accountName, role, inviteURL)
	accountName = strings.TrimSpace(accountName)
	role = strings.TrimSpace(role)
	if accountName == "" {
		accountName = "a Cogitor IoT account"
	}
	htmlOut, err = renderHTML(emailContent{
		Title:     "You are invited",
		Lead:      "You have been invited to join " + accountName,
		Badge:     role,
		Body:      "Open this link to accept and join the account.",
		LogoURL:   template.URL(logoSrc(publicBase)),
		LinkHTML:  linkHTML(inviteURL),
		FinePrint: "If you did not expect this invitation, you can ignore this email.",
	})
	return text, htmlOut, err
}
