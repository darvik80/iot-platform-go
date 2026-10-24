package mail

import (
	"bytes"
	"html/template"
	"net/url"
	"strings"
)

type emailContent struct {
	Preheader string
	Title     string
	Lead      string
	Body      string
	Badge     string
	Button    string
	URL       string
	Href      template.URL
	FinePrint string
}

// mailLink returns the URL shown in the message and the same value as a
// trusted href. html/template rejects scheme-less strings in href (the button
// would go to #ZgotmplZ while the text still showed the original URL).
func mailLink(raw string) (string, template.URL) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}
	u, err := url.Parse(s)
	if err != nil {
		return s, ""
	}
	if u.Scheme == "" {
		s = "https://" + strings.TrimPrefix(s, "//")
		u, err = url.Parse(s)
		if err != nil {
			return s, ""
		}
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return s, ""
	}
	return s, template.URL(s)
}

var emailTmpl = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="dark">
<meta name="supported-color-schemes" content="dark">
<title>{{.Title}}</title>
</head>
<body style="margin:0;padding:0;background-color:#0f1419;">
<span style="display:none!important;visibility:hidden;opacity:0;color:transparent;height:0;width:0;overflow:hidden;mso-hide:all;">{{.Preheader}}</span>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#0f1419;">
  <tr>
    <td align="center" style="padding:32px 16px;background-color:#0f1419;">
      <table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:560px;">
        <tr>
          <td style="padding:0 4px 24px 4px;">
            <table role="presentation" cellpadding="0" cellspacing="0" border="0">
              <tr>
                <td style="vertical-align:middle;padding-right:10px;">
                  <img src="cid:cogitor-logo" width="32" height="32" alt="" style="display:block;border:0;outline:none;text-decoration:none;border-radius:7px;">
                </td>
                <td style="vertical-align:middle;font-family:'IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:18px;font-weight:500;letter-spacing:0.02em;color:#e6edf3;line-height:1;">
                  Cogitor&nbsp;<span style="color:#3d9cf0;">IoT</span>
                </td>
              </tr>
            </table>
          </td>
        </tr>
        <tr>
          <td style="background-color:#1c2430;border:1px solid #2a3544;border-radius:8px;padding:28px 28px 24px 28px;">
            <h1 style="margin:0 0 12px 0;font-family:'IBM Plex Sans',Segoe UI,Helvetica,Arial,sans-serif;font-size:22px;line-height:1.3;font-weight:600;color:#e6edf3;">{{.Title}}</h1>
            {{if .Lead}}<p style="margin:0 0 16px 0;font-family:'IBM Plex Sans',Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.55;color:#e6edf3;">{{.Lead}}{{if .Badge}} as <span style="display:inline-block;background-color:rgba(61,156,240,0.15);color:#3d9cf0;border-radius:4px;padding:2px 8px;font-family:'IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:12px;letter-spacing:0.02em;">{{.Badge}}</span>{{end}}</p>{{end}}
            {{if .Body}}<p style="margin:0 0 24px 0;font-family:'IBM Plex Sans',Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.55;color:#8b9bb0;">{{.Body}}</p>{{end}}
            <table role="presentation" cellpadding="0" cellspacing="0" border="0">
              <tr>
                <td bgcolor="#3d9cf0" style="border-radius:6px;background-color:#3d9cf0;">
                  <a href="{{.Href}}" style="display:inline-block;padding:12px 22px;font-family:'IBM Plex Sans',Segoe UI,Helvetica,Arial,sans-serif;font-size:14px;font-weight:600;line-height:1;color:#0f1419;text-decoration:none;">{{.Button}}</a>
                </td>
              </tr>
            </table>
            <p style="margin:20px 0 0 0;font-family:'IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:12px;line-height:1.5;word-break:break-all;">
              <a href="{{.Href}}" style="color:#8b9bb0;text-decoration:underline;">{{.URL}}</a>
            </p>
          </td>
        </tr>
        <tr>
          <td style="padding:20px 4px 0 4px;font-family:'IBM Plex Sans',Segoe UI,Helvetica,Arial,sans-serif;font-size:12px;line-height:1.5;color:#8b9bb0;">
            {{.FinePrint}}
          </td>
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

func verificationContent(login, verifyURL string) (text string, html string, err error) {
	login = strings.TrimSpace(login)
	hello := "Hello,"
	if login != "" {
		hello = "Hello " + login + ","
	}
	display, href := mailLink(verifyURL)
	text = hello + "\n\nConfirm your Cogitor IoT account by opening this link:\n" + display + "\n\nIf you did not register, ignore this email.\n"
	html, err = renderHTML(emailContent{
		Preheader: "Confirm your Cogitor IoT account.",
		Title:     "Confirm your account",
		Lead:      hello,
		Body:      "Use the button below to confirm this email for your Cogitor IoT account.",
		Button:    "Confirm account",
		URL:       display,
		Href:      href,
		FinePrint: "If you did not register, you can ignore this email.",
	})
	return text, html, err
}

func invitationContent(accountName, role, inviteURL string) (text string, html string, err error) {
	accountName = strings.TrimSpace(accountName)
	role = strings.TrimSpace(role)
	if accountName == "" {
		accountName = "a Cogitor IoT account"
	}
	text = "You have been invited to join " + accountName
	if role != "" {
		text += " as " + role
	}
	display, href := mailLink(inviteURL)
	text += ".\n\nAccept the invitation by opening this link:\n" + display + "\n\nIf you did not expect this, ignore this email.\n"
	html, err = renderHTML(emailContent{
		Preheader: "Join " + accountName + " on Cogitor IoT.",
		Title:     "You're invited",
		Lead:      "You have been invited to join " + accountName,
		Badge:     role,
		Body:      "Use the button below to accept and join the account.",
		Button:    "Accept invitation",
		URL:       display,
		Href:      href,
		FinePrint: "If you did not expect this invitation, you can ignore this email.",
	})
	return text, html, err
}
