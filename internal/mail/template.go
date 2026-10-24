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
	Href      template.URL
	Button    string
	CopyURL   template.HTML
	FinePrint string
}

func safeHTTPURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	if u.Scheme == "" {
		s = "https://" + strings.TrimPrefix(s, "//")
		u, err = url.Parse(s)
		if err != nil {
			return ""
		}
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return s
}

// copyableURL renders an address that people can copy, without a consecutive
// "https://" that mail clients autolink and wrap in extra quotes.
func copyableURL(u string) template.HTML {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	rest := u
	prefix := ""
	switch {
	case strings.HasPrefix(u, "https://"):
		prefix, rest = "https:", strings.TrimPrefix(u, "https:")
	case strings.HasPrefix(u, "http://"):
		prefix, rest = "http:", strings.TrimPrefix(u, "http:")
	}
	return template.HTML(html.EscapeString(prefix) + `<span></span>` + html.EscapeString(rest))
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
            Cogitor <span style="color:#3d9cf0;">IoT</span>
          </td>
        </tr>
        <tr>
          <td style="background-color:#1c2430;border:1px solid #2a3544;padding:28px 24px;">
            <h1 style="margin:0 0 12px 0;font-family:Arial,Helvetica,sans-serif;font-size:22px;color:#e6edf3;">{{.Title}}</h1>
            {{if .Lead}}<p style="margin:0 0 16px 0;font-family:Arial,Helvetica,sans-serif;font-size:15px;line-height:1.5;color:#e6edf3;">{{.Lead}}{{if .Badge}} as {{.Badge}}{{end}}</p>{{end}}
            {{if .Body}}<p style="margin:0 0 24px 0;font-family:Arial,Helvetica,sans-serif;font-size:15px;line-height:1.5;color:#8b9bb0;">{{.Body}}</p>{{end}}
            {{if .Href}}<p style="margin:0 0 16px 0;font-family:Arial,Helvetica,sans-serif;font-size:16px;">
              <a href="{{.Href}}" style="color:#3d9cf0;">{{.Button}}</a>
            </p>
            <p style="margin:0 0 6px 0;font-family:Arial,Helvetica,sans-serif;font-size:12px;color:#8b9bb0;">Or copy this address:</p>
            <p style="margin:0;font-family:Arial,Helvetica,sans-serif;font-size:12px;line-height:1.5;color:#8b9bb0;word-break:break-all;">{{.CopyURL}}</p>{{end}}
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

func verificationContent(login, verifyURL string) (text string, htmlOut string, err error) {
	login = strings.TrimSpace(login)
	hello := "Hello,"
	if login != "" {
		hello = "Hello " + login + ","
	}
	link := safeHTTPURL(verifyURL)
	text = hello + "\n\nConfirm your Cogitor IoT account by opening this link:\n" + link + "\n\nIf you did not register, ignore this email.\n"
	htmlOut, err = renderHTML(emailContent{
		Title:     "Confirm your account",
		Lead:      hello,
		Body:      "Open the link below to confirm this email for your Cogitor IoT account.",
		Href:      template.URL(link),
		Button:    "Confirm account",
		CopyURL:   copyableURL(link),
		FinePrint: "If you did not register, you can ignore this email.",
	})
	return text, htmlOut, err
}

func invitationContent(accountName, role, inviteURL string) (text string, htmlOut string, err error) {
	accountName = strings.TrimSpace(accountName)
	role = strings.TrimSpace(role)
	if accountName == "" {
		accountName = "a Cogitor IoT account"
	}
	link := safeHTTPURL(inviteURL)
	text = "You have been invited to join " + accountName
	if role != "" {
		text += " as " + role
	}
	text += ".\n\nAccept the invitation:\n" + link + "\n\nIf you did not expect this, ignore this email.\n"
	htmlOut, err = renderHTML(emailContent{
		Title:     "You are invited",
		Lead:      "You have been invited to join " + accountName,
		Badge:     role,
		Body:      "Open the link below to accept and join the account.",
		Href:      template.URL(link),
		Button:    "Accept invitation",
		CopyURL:   copyableURL(link),
		FinePrint: "If you did not expect this invitation, you can ignore this email.",
	})
	return text, htmlOut, err
}
