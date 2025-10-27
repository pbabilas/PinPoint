package internal

import (
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
)

const (
	expirationDays = 30
)

type Mailer struct {
	logger *logrus.Logger
}

func NewMailer(logger *logrus.Logger) *Mailer {
	return &Mailer{logger: logger}
}

func (m *Mailer) SendEmail(fileContent string, expirationDays int, emailTemplate string, name string, address string) (err error) {
	mailer := gomail.NewMessage()
	mailer.SetHeader("From", os.Getenv("SMTP_FROM"))
	mailer.SetHeader("To", address)
	mailer.SetHeader("Subject", "Nowa konfiguracja OpenVPN dla B-Code")

	mailer.SetBody("text/html", fmt.Sprintf(emailTemplate, expirationDays))
	mailer.Attach(fmt.Sprintf("%s.ovpn", name), gomail.SetCopyFunc(func(w io.Writer) error {
		_, err := w.Write([]byte(fileContent))
		return err
	}))
	d := gomail.NewDialer(os.Getenv("SMTP_HOST"), 587, os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD"))
	if err := d.DialAndSend(mailer); err != nil {
		return fmt.Errorf("Błąd podczas wysyłania e-maila: %v", err)
	}

	return nil
}
