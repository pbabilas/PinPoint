package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"b-code.cloud/routeros/ovpn/internal"
	"github.com/akamensky/argparse"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

//go:embed user.ovpn.template mail.template.html
var config embed.FS

func main() {

	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}

	parser := argparse.NewParser("vault-ovpn-renew", "Renew OpenVPN certificates from HashiCorp Vault")
	commonName := parser.String("n", "name", &argparse.Options{Required: false, Help: "Certificate common name", Default: "ovpn-pbabilas"})
	email := parser.String("e", "email", &argparse.Options{Required: false, Help: "Recipient address", Default: nil})
	serialNumber := parser.String("s", "serial", &argparse.Options{Required: false, Help: "Current certificate serial number (if checking existing cert)", Default: ""})
	ttl := parser.String("t", "ttl", &argparse.Options{Required: false, Help: "Certificate TTL", Default: "8760h"}) // 1 year
	outputDir := parser.String("o", "output-dir", &argparse.Options{Required: false, Help: "Relative config output directory", Default: "conf"})

	logger := &logrus.Logger{
		Out:          os.Stderr,
		Formatter:    new(logrus.JSONFormatter),
		Hooks:        make(logrus.LevelHooks),
		Level:        logrus.InfoLevel,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}

	if err = parser.Parse(os.Args); err != nil {
		logger.Fatalf(parser.Usage(err))
	}

	info, err := os.Stat(*outputDir)
	if err != nil {
		log.Fatalf(err.Error())
	}
	if !info.IsDir() {
		log.Fatalf(fmt.Sprintf("Output directory '%s' should be a directory", *outputDir))
	}

	// Pobierz konfigurację z ENV
	vaultAddr := os.Getenv("VAULT_ADDR")
	vaultRoleID := os.Getenv("VAULT_ROLE_ID")
	vaultSecretID := os.Getenv("VAULT_SECRET_ID")
	vaultPKIPath := os.Getenv("VAULT_PKI_PATH")
	vaultRole := os.Getenv("VAULT_ROLE")

	if vaultAddr == "" || vaultRoleID == "" || vaultSecretID == "" || vaultPKIPath == "" || vaultRole == "" {
		log.Fatalf("Brak wymaganej konfiguracji Vault. Sprawdź zmienne: VAULT_ADDR, VAULT_ROLE_ID, VAULT_SECRET_ID, VAULT_PKI_PATH, VAULT_ROLE")
	}

	// Utwórz klienta Vault
	vaultClient, err := internal.NewVaultClient(vaultAddr, vaultRoleID, vaultSecretID, vaultPKIPath, vaultRole, logger)
	if err != nil {
		log.Fatalf("Błąd podczas tworzenia klienta Vault: %v", err)
	}

	logger.Infof("Połączono z Vault: %s", vaultAddr)

	var certInfo *internal.CertificateInfo
	var needsRenewal bool
	var daysUntilExpiry float64

	// Jeśli podano serial number, sprawdź istniejący certyfikat
	if *serialNumber != "" {
		logger.Infof("Sprawdzanie istniejącego certyfikatu: serial=%s", *serialNumber)
		certInfo, err = vaultClient.GetCertificateInfo(*serialNumber)
		if err != nil {
			log.Fatalf("Błąd podczas pobierania informacji o certyfikacie: %v", err)
		}

		needsRenewal, daysUntilExpiry = vaultClient.CheckCertificateExpiration(certInfo, 30)
		logger.Infof("Certyfikat wygasa za %.1f dni", daysUntilExpiry)

		if needsRenewal {
			logger.Warnf("Certyfikat wymaga odnowienia (< 30 dni do wygaśnięcia)")
			// Odnów certyfikat
			certInfo, err = vaultClient.RenewCertificate(*serialNumber, *commonName, *ttl)
			if err != nil {
				log.Fatalf("Błąd podczas odnawiania certyfikatu: %v", err)
			}
		} else {
			logger.Infof("Certyfikat nie wymaga odnowienia")
			// Pobierz dane certyfikatu do wysłania
			// (certInfo jest już wypełniony, ale nie ma private key - trzeba wygenerować nowy)
			logger.Warnf("UWAGA: Nie można pobrać klucza prywatnego dla istniejącego certyfikatu z Vault")
			logger.Warnf("Generuję nowy certyfikat...")
			certInfo, err = vaultClient.IssueCertificate(*commonName, *ttl)
			if err != nil {
				log.Fatalf("Błąd podczas generowania nowego certyfikatu: %v", err)
			}
		}
	} else {
		// Brak serial number - wygeneruj nowy certyfikat
		logger.Infof("Generowanie nowego certyfikatu dla %s", *commonName)
		certInfo, err = vaultClient.IssueCertificate(*commonName, *ttl)
		if err != nil {
			log.Fatalf("Błąd podczas generowania certyfikatu: %v", err)
		}
		daysUntilExpiry = certInfo.ExpiresAt.Sub(certInfo.ExpiresAt.AddDate(0, 0, -365)).Hours() / 24
	}

	// Pobierz certyfikat CA
	caCert, err := vaultClient.GetCACertificate()
	if err != nil {
		log.Fatalf("Błąd podczas pobierania certyfikatu CA: %v", err)
	}

	// Wczytaj szablon konfiguracji OpenVPN
	var ovpnTemplate []byte
	if ovpnTemplate, err = config.ReadFile("user.ovpn.template"); err != nil {
		log.Fatalf("Błąd podczas odczytu pliku szablonu: %v", err)
	}

	// Wygeneruj konfigurację OpenVPN
	ovpnConfig := fmt.Sprintf(string(ovpnTemplate), caCert, certInfo.Certificate, certInfo.PrivateKey)
	err = os.WriteFile(fmt.Sprintf("%s/%s.ovpn", *outputDir, *commonName), []byte(ovpnConfig), 0644)
	if err != nil {
		log.Fatalf(err.Error())
	}

	if *email != "" {
		var emailTemplate []byte
		if emailTemplate, err = config.ReadFile("mail.template.html"); err != nil {
			log.Fatalf("Błąd podczas odczytu szablonu email: %v", err)
		}

		mailer := internal.NewMailer(logger)
		if err = mailer.SendEmail(ovpnConfig, daysUntilExpiry, string(emailTemplate), *commonName, *email); err != nil {
			log.Fatalf("Błąd podczas wysyłania e-maila: %v", err)
		}
		logger.Infof("Konfiguracja OpenVPN została wysłana na e-mail")
	}

	logger.Infof("Serial number nowego certyfikatu: %s", certInfo.SerialNumber)
	logger.Infof("Zapisz ten serial number do przyszłego użycia!")
}
