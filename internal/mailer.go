package internal

import (
	"embed"
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
	"io"
	"log"
	"os"
)

var template embed.FS

const (
	expirationDays = 30
)

type Mailer struct {
	logger *logrus.Logger
}

func NewMailer(logger *logrus.Logger) *Mailer {
	return &Mailer{logger: logger}
}

func (m *Mailer) SendEmail(fileContent string, expirationDays float64) (err error) {
	mailer := gomail.NewMessage()
	mailer.SetHeader("From", os.Getenv("SMTP_FROM"))
	mailer.SetHeader("To", os.Getenv("SMTP_TO"))
	mailer.SetHeader("Subject", "Nowa konfiguracja OpenVPN dla B-Code")
	var body []byte
	if body, err = template.ReadFile("mail.template.html"); err != nil {
		log.Fatalf("Błąd podczas odczytu pliku: %v", err)
	}
	mailer.SetBody("text/plain", fmt.Sprintf(string(body), expirationDays))
	mailer.Attach("pbabilas.ovpn", gomail.SetCopyFunc(func(w io.Writer) error {
		_, err := w.Write([]byte(fileContent))
		return err
	}))
	d := gomail.NewDialer(os.Getenv("SMTP_HOST"), 587, os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD"))
	if err := d.DialAndSend(mailer); err != nil {
		return fmt.Errorf("Błąd podczas wysyłania e-maila: %v", err)
	}

	return nil
}
