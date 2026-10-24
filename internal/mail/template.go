package mail

import (
	"bytes"
	"html"
	"html/template"
	"strings"
)

type emailContent struct {
	Title       string
	Lead        string
	Body        string
	Badge       string
	LogoURL     template.URL
	ButtonURL   template.URL
	ButtonText  string
	FallbackURL template.HTML
	FinePrint   string
}

func logoSrc(publicBase string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return ""
	}
	return base + "/favicon.png"
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
          <td style="padding:0 0 24px 0;font-family:Arial,Helvetica,sans-serif;font-size:20px;font-weight:600;color:#e6edf3;letter-spacing:0.3px;">
            {{if .LogoURL}}<img src="{{.LogoURL}}" width="24" height="24" alt="" style="width:24px;height:24px;border:0;vertical-align:middle;margin-right:10px;border-radius:4px;">{{end}}Cogitor <span style="color:#3d9cf0;font-weight:600;">IoT</span>
          </td>
        </tr>
        <tr>
          <td style="background-color:#1c2430;border:1px solid #2a3544;border-radius:8px;padding:32px 28px;">
            <h1 style="margin:0 0 16px 0;font-family:Arial,Helvetica,sans-serif;font-size:22px;font-weight:600;color:#e6edf3;">{{.Title}}</h1>
            {{if .Lead}}<p style="margin:0 0 8px 0;font-family:Arial,Helvetica,sans-serif;font-size:15px;line-height:1.6;color:#e6edf3;">{{.Lead}}{{if .Badge}} <span style="display:inline-block;background-color:rgba(61,156,240,0.15);color:#3d9cf0;font-size:12px;font-weight:600;padding:2px 10px;border-radius:10px;margin-left:4px;">{{.Badge}}</span>{{end}}</p>{{end}}
            {{if .Body}}<p style="margin:0 0 24px 0;font-family:Arial,Helvetica,sans-serif;font-size:15px;line-height:1.6;color:#8b9bb0;">{{.Body}}</p>{{end}}
            {{if .ButtonURL}}
            <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 24px 0;">
              <tr>
                <td>
                  <a href="{{.ButtonURL}}" style="display:inline-block;background-color:#3d9cf0;border-radius:6px;padding:12px 32px;font-family:Arial,Helvetica,sans-serif;font-size:15px;font-weight:600;color:#ffffff;text-decoration:none;">{{.ButtonText}}</a>
                </td>
              </tr>
            </table>
            {{end}}
            {{if .FallbackURL}}<p style="margin:0;font-family:Arial,Helvetica,sans-serif;font-size:13px;line-height:1.5;color:#8b9bb0;word-break:break-all;">Or copy this link into your browser:<br>{{.FallbackURL}}</p>{{end}}
          </td>
        </tr>
        <tr>
          <td style="padding:20px 0 0 0;font-family:Arial,Helvetica,sans-serif;font-size:12px;line-height:1.5;color:#5a6a7e;">{{.FinePrint}}</td>
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
	esc := html.EscapeString(strings.TrimSpace(verifyURL))
	htmlOut, err = renderHTML(emailContent{
		Title:       "Confirm your account",
		Lead:        hello,
		Body:        "Confirm this email address for your Cogitor IoT account.",
		LogoURL:     template.URL(logoSrc(publicBase)),
		ButtonURL:   template.URL(verifyURL),
		ButtonText:  "Confirm account",
		FallbackURL: template.HTML(`<a href="` + esc + `">` + esc + `</a>`),
		FinePrint:   "If you did not register, you can ignore this email.",
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
	esc := html.EscapeString(strings.TrimSpace(inviteURL))
	htmlOut, err = renderHTML(emailContent{
		Title:       "You are invited",
		Lead:        "You have been invited to join " + accountName,
		Badge:       role,
		Body:        "Accept the invitation to get access to the account.",
		LogoURL:     template.URL(logoSrc(publicBase)),
		ButtonURL:   template.URL(inviteURL),
		ButtonText:  "Accept invitation",
		FallbackURL: template.HTML(`<a href="` + esc + `">` + esc + `</a>`),
		FinePrint:   "If you did not expect this invitation, you can ignore this email.",
	})
	return text, htmlOut, err
}
