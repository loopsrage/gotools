package emaillib

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"unicode"
)

// SMTPConfig configures SMTP delivery (Gmail STARTTLS :587 or SMTPS :465).
type SMTPConfig struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"-" yaml:"password"`
	From     string `json:"from" yaml:"from"`
	UseTLS   bool   `json:"use_tls" yaml:"use_tls"`
}

// DefaultGmailConfig returns Gmail STARTTLS defaults on port 587.
func DefaultGmailConfig(username, appPassword string) SMTPConfig {
	return SMTPConfig{
		Host:     "smtp.gmail.com",
		Port:     587,
		Username: username,
		Password: appPassword,
		From:     username,
		UseTLS:   true,
	}
}

// Sender dispatches plain-text/HTML emails and Google Fi SMS gateways via SMTP.
type Sender struct {
	Config SMTPConfig
}

// NewSender creates an SMTP Sender.
func NewSender(cfg SMTPConfig) *Sender {
	if cfg.Host == "" {
		cfg.Host = "smtp.gmail.com"
	}
	if cfg.Port <= 0 {
		cfg.Port = 587
	}
	if cfg.From == "" {
		cfg.From = cfg.Username
	}
	return &Sender{Config: cfg}
}

var reTemplateVar = regexp.MustCompile(`\{([a-zA-Z0-9_.-]+)\}`)

// RenderTemplate safely substitutes `{key}` placeholders using vars without executing arbitrary code.
// Unknown placeholders are preserved verbatim.
func RenderTemplate(tmpl string, vars map[string]any) string {
	if len(vars) == 0 {
		return tmpl
	}
	return reTemplateVar.ReplaceAllStringFunc(tmpl, func(match string) string {
		key := match[1 : len(match)-1]
		if v, ok := vars[key]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return match
	})
}

// FormatGoogleFiSMSAddress converts a US/international phone number to Google Fi's
// email-to-SMS gateway (`<digits>@msg.fi.google.com`).
func FormatGoogleFiSMSAddress(phone string) (string, error) {
	var digits strings.Builder
	for _, r := range phone {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	if len(d) == 11 && strings.HasPrefix(d, "1") {
		d = d[1:]
	}
	if len(d) < 10 {
		return "", fmt.Errorf("emaillib: invalid phone number %q", phone)
	}
	return d + "@msg.fi.google.com", nil
}

// BuildMessage formats RFC 5322 headers and body.
func BuildMessage(from string, to []string, subject, body string, isHTML bool) []byte {
	ct := "text/plain; charset=UTF-8"
	if isHTML {
		ct = "text/html; charset=UTF-8"
	}
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: " + ct + "\r\n\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// Send dispatches an email via SMTP (STARTTLS on port 587 or implicit TLS on port 465).
func (s *Sender) Send(to []string, subject, body string, isHTML bool) error {
	if len(to) == 0 {
		return fmt.Errorf("emaillib.Send: at least one recipient required")
	}
	addr := net.JoinHostPort(s.Config.Host, fmt.Sprintf("%d", s.Config.Port))
	msg := BuildMessage(s.Config.From, to, subject, body, isHTML)

	if s.Config.Port == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.Config.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		client, err := smtp.NewClient(conn, s.Config.Host)
		if err != nil {
			_ = conn.Close()
			return err
		}
		defer client.Close()
		if s.Config.Username != "" {
			if err := client.Auth(smtp.PlainAuth("", s.Config.Username, s.Config.Password, s.Config.Host)); err != nil {
				return err
			}
		}
		if err := client.Mail(s.Config.From); err != nil {
			return err
		}
		for _, rcpt := range to {
			if err := client.Rcpt(rcpt); err != nil {
				return err
			}
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write(msg); err != nil {
			_ = w.Close()
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return client.Quit()
	}

	var auth smtp.Auth
	if s.Config.Username != "" {
		auth = smtp.PlainAuth("", s.Config.Username, s.Config.Password, s.Config.Host)
	}
	return smtp.SendMail(addr, auth, s.Config.From, to, msg)
}

// SendGoogleFiSMS sends a short text message via Google Fi's email-to-SMS gateway.
func (s *Sender) SendGoogleFiSMS(phone, text string) error {
	addr, err := FormatGoogleFiSMSAddress(phone)
	if err != nil {
		return err
	}
	return s.Send([]string{addr}, "", text, false)
}
