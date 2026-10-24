package mail

import (
	"bytes"
	"html"
	"html/template"
	"net/url"
	"strings"
)

const emailFont = `Arial, Helvetica, sans-serif`

type emailContent struct {
	Title      string
	Lead       string
	Body       string
	Badge      string
	ButtonHTML template.HTML
	AltHTML    template.HTML
	FinePrint  string
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

func hrefHTML(href, label, style string) template.HTML {
	label = html.EscapeString(label)
	if href == "" {
		return template.HTML(label)
	}
	return template.HTML(`<a href="` + html.EscapeString(href) + `" style="` + style + `">` + label + `</a>`)
}

// buttonHTML is a bulletproof email button: the <a href> IS the coloured
// control, so a click anywhere on "Accept invitation" follows the invite URL.
func buttonHTML(href, label string) template.HTML {
	label = html.EscapeString(label)
	if href == "" {
		return template.HTML(label)
	}
	return template.HTML(
		`<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>` +
			`<td align="center" bgcolor="#3d9cf0" style="border-radius:6px;background-color:#3d9cf0;">` +
			`<a href="` + html.EscapeString(href) + `" style="display:inline-block;padding:12px 24px;font-family:` + emailFont + `;font-size:16px;font-weight:bold;line-height:20px;color:#0f1419;text-decoration:none;border:1px solid #3d9cf0;border-radius:6px;background-color:#3d9cf0;">` +
			label + `</a></td></tr></table>`)
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
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#0f1419;">
  <tr>
    <td align="center" style="padding:32px 16px;background-color:#0f1419;">
      <table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:560px;">
        <tr>
          <td style="padding:0 4px 24px 4px;">
            <table role="presentation" cellpadding="0" cellspacing="0" border="0">
              <tr>
                <td style="vertical-align:middle;font-family:Arial, Helvetica, sans-serif;font-size:18px;font-weight:bold;color:#e6edf3;line-height:1;">
                  Cogitor <span style="color:#3d9cf0;">IoT</span>
                </td>
              </tr>
            </table>
          </td>
        </tr>
        <tr>
          <td style="background-color:#1c2430;border:1px solid #2a3544;border-radius:8px;padding:28px 24px 24px 24px;">
            <h1 style="margin:0 0 12px 0;font-family:Arial, Helvetica, sans-serif;font-size:22px;line-height:1.3;font-weight:bold;color:#e6edf3;">{{.Title}}</h1>
            {{if .Lead}}<p style="margin:0 0 16px 0;font-family:Arial, Helvetica, sans-serif;font-size:15px;line-height:1.55;color:#e6edf3;">{{.Lead}}{{if .Badge}} as <span style="display:inline-block;background-color:#1a3348;color:#3d9cf0;border-radius:4px;padding:2px 8px;font-family:Arial, Helvetica, sans-serif;font-size:12px;">{{.Badge}}</span>{{end}}</p>{{end}}
            {{if .Body}}<p style="margin:0 0 24px 0;font-family:Arial, Helvetica, sans-serif;font-size:15px;line-height:1.55;color:#8b9bb0;">{{.Body}}</p>{{end}}
            {{.ButtonHTML}}
            {{if .AltHTML}}<p style="margin:20px 0 0 0;font-family:Arial, Helvetica, sans-serif;font-size:12px;line-height:1.5;color:#8b9bb0;">If the button does not open, {{.AltHTML}}.</p>{{end}}
          </td>
        </tr>
        <tr>
          <td style="padding:20px 4px 0 4px;font-family:Arial, Helvetica, sans-serif;font-size:12px;line-height:1.5;color:#8b9bb0;">
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

func verificationContent(login, verifyURL string) (text string, htmlOut string, err error) {
	login = strings.TrimSpace(login)
	hello := "Hello,"
	if login != "" {
		hello = "Hello " + login + ","
	}
	link := safeHTTPURL(verifyURL)
	if link == "" {
		link = strings.TrimSpace(verifyURL)
	}
	text = hello + "\n\nConfirm your Cogitor IoT account by opening this link:\n" + link + "\n\nIf you did not register, ignore this email.\n"
	htmlOut, err = renderHTML(emailContent{
		Title:      "Confirm your account",
		Lead:       hello,
		Body:       "Use the button below to confirm this email for your Cogitor IoT account.",
		ButtonHTML: buttonHTML(safeHTTPURL(verifyURL), "Confirm account"),
		AltHTML:    hrefHTML(safeHTTPURL(verifyURL), "open this page", "color:#3d9cf0;text-decoration:underline;"),
		FinePrint:  "If you did not register, you can ignore this email.",
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
	if link == "" {
		link = strings.TrimSpace(inviteURL)
	}
	text = "You have been invited to join " + accountName
	if role != "" {
		text += " as " + role
	}
	text += ".\n\nAccept the invitation by opening this link:\n" + link + "\n\nIf you did not expect this, ignore this email.\n"
	htmlOut, err = renderHTML(emailContent{
		Title:      "You are invited",
		Lead:       "You have been invited to join " + accountName,
		Badge:      role,
		Body:       "Use the button below to accept and join the account.",
		ButtonHTML: buttonHTML(safeHTTPURL(inviteURL), "Accept invitation"),
		AltHTML:    hrefHTML(safeHTTPURL(inviteURL), "open this page", "color:#3d9cf0;text-decoration:underline;"),
		FinePrint:  "If you did not expect this invitation, you can ignore this email.",
	})
	return text, htmlOut, err
}
