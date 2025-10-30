package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pbabilas/pinpoint/internal"
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
	ttl := parser.String("t", "ttl", &argparse.Options{Required: false, Help: "Certificate TTL", Default: "8760h"}) // 1 year
	outputDir := parser.String("o", "output-dir", &argparse.Options{Required: false, Help: "Relative config output directory", Default: "conf"})
	certDBPath := parser.String("d", "cert-db", &argparse.Options{Required: false, Help: "Certificate database file path", Default: "certificates.json"})
	forceRenew := parser.Flag("f", "force-renew", &argparse.Options{Required: false, Help: "Force certificate renewal even if not expired"})
	resendEmail := parser.Flag("r", "resend", &argparse.Options{Required: false, Help: "Resend email even if certificate was not renewed"})
	mode := parser.String("m", "mode", &argparse.Options{Required: false, Help: "Operation mode: client or server", Default: "client"})
	mikrotikIP := parser.String("i", "mikrotik-ip", &argparse.Options{Required: false, Help: "Mikrotik router IP address (server mode only)"})

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
	vaultServerRole := os.Getenv("VAULT_SERVER_ROLE")
	if vaultServerRole == "" {
		vaultServerRole = "ovpn-server" // Domyślna rola serwera
	}

	if vaultAddr == "" || vaultRoleID == "" || vaultSecretID == "" || vaultPKIPath == "" || vaultRole == "" {
		log.Fatalf("Brak wymaganej konfiguracji Vault. Sprawdź zmienne: VAULT_ADDR, VAULT_ROLE_ID, VAULT_SECRET_ID, VAULT_PKI_PATH, VAULT_ROLE")
	}

	// Utwórz klienta Vault
	vaultClient, err := internal.NewVaultClient(vaultAddr, vaultRoleID, vaultSecretID, vaultPKIPath, vaultRole, vaultServerRole, logger)
	if err != nil {
		log.Fatalf("Błąd podczas tworzenia klienta Vault: %v", err)
	}

	logger.Infof("Połączono z Vault: %s", vaultAddr)

	// Wczytaj bazę danych certyfikatów
	certDB, err := internal.LoadCertificateDB(*certDBPath, logger)
	if err != nil {
		log.Fatalf("Błąd podczas wczytywania bazy danych certyfikatów: %v", err)
	}

	var certInfo *internal.CertificateInfo
	var needsRenewal bool
	var daysUntilExpiry float64
	var userCert *internal.UserCertificate
	var userExists bool
	var certificateRenewed bool

	// Sprawdź tryb pracy
	if *mode == "server" {
		logger.Infof("Uruchomiono w trybie serwera")
		handleServerMode(certDB, vaultClient, logger, *commonName, *email, *ttl, *outputDir, *mikrotikIP, *forceRenew, *resendEmail)
		return
	}

	// Tryb klienta (domyślny)
	logger.Infof("Uruchomiono w trybie klienta")

	// Sprawdź czy użytkownik istnieje w bazie danych
	userCert, userExists = certDB.GetUser(*commonName)

	if userExists {
		logger.Infof("Znaleziono użytkownika w bazie: %s, serial: %s", *commonName, userCert.SerialNumber)

		// Aktualizuj email w bazie danych, jeśli podano nowy
		if *email != "" && *email != userCert.Email {
			userCert.Email = *email
			err = certDB.AddOrUpdateUser(*userCert)
			if err != nil {
				logger.Warnf("Błąd podczas aktualizacji emaila w bazie danych: %v", err)
			} else {
				logger.Infof("Zaktualizowano email dla użytkownika %s: %s", *commonName, *email)
			}
		}

		// Sprawdź ważność istniejącego certyfikatu
		var err error
		needsRenewal, daysUntilExpiry, err = certDB.CheckCertificateExpiry(*commonName, 30)
		if err != nil {
			log.Fatalf("Błąd podczas sprawdzania ważności certyfikatu: %v", err)
		}
		logger.Infof("Certyfikat wygasa za %.1f dni", daysUntilExpiry)

		if needsRenewal || *forceRenew {
			certificateRenewed = true
			if *forceRenew && !needsRenewal {
				logger.Warnf("Wymuszono odnowienie certyfikatu (opcja --force-renew)")
			} else {
				logger.Warnf("Certyfikat wymaga odnowienia (< 30 dni do wygaśnięcia)")
			}
			// Odnów certyfikat
			certInfo, err = vaultClient.RenewCertificate(userCert.SerialNumber, *commonName, *ttl)
			if err != nil {
				log.Fatalf("Błąd podczas odnawiania certyfikatu: %v", err)
			}

			// Zaktualizuj bazę danych z nowym numerem seryjnym
			err = certDB.UpdateCertificateInfo(*commonName, certInfo.SerialNumber, certInfo.ExpiresAt)
			if err != nil {
				logger.Warnf("Błąd podczas aktualizacji bazy danych: %v", err)
			}
		} else {
			certificateRenewed = false
			logger.Infof("Certyfikat nie wymaga odnowienia - jest jeszcze ważny przez %.1f dni", daysUntilExpiry)
			logger.Infof("Nie generuję nowego certyfikatu, ponieważ obecny jest jeszcze ważny")

			// Pobierz informacje o istniejącym certyfikacie z Vault
			certInfo, err = vaultClient.GetCertificateInfo(userCert.SerialNumber)
			if err != nil {
				log.Fatalf("Błąd podczas pobierania informacji o certyfikacie: %v", err)
			}

			// Ustawiamy datę wygaśnięcia z bazy danych, ponieważ certInfo z Vault nie ma tej informacji
			certInfo.ExpiresAt = userCert.ExpiresAt
		}
	} else {
		// Użytkownik nie istnieje - wygeneruj nowy certyfikat
		logger.Infof("Generowanie nowego certyfikatu dla nowego użytkownika %s", *commonName)
		certInfo, err = vaultClient.IssueCertificate(*commonName, *ttl)
		if err != nil {
			log.Fatalf("Błąd podczas generowania certyfikatu: %v", err)
		}

		// Dodaj użytkownika do bazy danych
		newUserCert := internal.UserCertificate{
			CommonName:   *commonName,
			SerialNumber: certInfo.SerialNumber,
			Email:        *email,
			CreatedAt:    time.Now(),
			LastRenewed:  time.Now(),
			ExpiresAt:    certInfo.ExpiresAt,
			TTL:          *ttl,
		}

		err = certDB.AddOrUpdateUser(newUserCert)
		if err != nil {
			logger.Warnf("Błąd podczas dodawania użytkownika do bazy danych: %v", err)
		}

		daysUntilExpiry = certInfo.ExpiresAt.Sub(time.Now()).Hours() / 24

		// Sprawdź czy istnieje certyfikat serwera dla tego użytkownika
		if _, exists := certDB.GetServerCertificate(*commonName); exists {
			logger.Infof("Znaleziono certyfikat serwera dla %s, aktualizuję konfigurację Mikrotika", *commonName)

			// Tutaj byłaby logika aktualizacji certyfikatu serwera na Mikrotiku
			// W przyszłości można zaimplementować API do Mikrotika
			logger.Infof("Certyfikat serwera dla %s wymaga aktualizacji na routerze Mikrotik", *commonName)
		}
	}

	// Wczytaj szablon konfiguracji OpenVPN
	var ovpnTemplate []byte
	if ovpnTemplate, err = config.ReadFile("user.ovpn.template"); err != nil {
		log.Fatalf("Błąd podczas odczytu pliku szablonu: %v", err)
	}

	// Sprawdź czy mamy klucz prywatny (tylko dla nowo wygenerowanych certyfikatów)
	var ovpnConfig string
	if certInfo.PrivateKey != "" {
		// Mamy klucz prywatny - generujemy nową konfigurację
		ovpnConfig = fmt.Sprintf(string(ovpnTemplate), certInfo.CAChain, certInfo.Certificate, certInfo.PrivateKey)
		err = os.WriteFile(fmt.Sprintf("%s/%s.ovpn", *outputDir, *commonName), []byte(ovpnConfig), 0644)
		if err != nil {
			log.Fatalf(err.Error())
		}
		logger.Infof("Wygenerowano nową konfigurację OpenVPN")
	} else {
		// Nie mamy klucza prywatnego - sprawdzamy czy istnieje plik konfiguracyjny
		configPath := fmt.Sprintf("%s/%s.ovpn", *outputDir, *commonName)
		if configData, err := os.ReadFile(configPath); err == nil {
			ovpnConfig = string(configData)
			logger.Infof("Użyto istniejącej konfiguracji OpenVPN z pliku: %s", configPath)
		} else {
			// Nie mamy konfiguracji i nie możemy jej wygenerować bez klucza prywatnego
			logger.Warnf("Nie można wygenerować konfiguracji OpenVPN - brak klucza prywatnego dla istniejącego certyfikatu")
			logger.Warnf("Aby wygenerować nową konfigurację, użyj opcji --force-renew")
			logger.Infof("Certyfikat jest jeszcze ważny przez %.1f dni", daysUntilExpiry)
		}
	}

	// Pobierz email z bazy danych, jeśli nie podano w parametrze
	userEmail := *email
	if userEmail == "" && userExists {
		userEmail = userCert.Email
	}

	// Wysyłaj email tylko jeśli certyfikat został odnowiony lub użyto flagi --resend
	if (certificateRenewed || *resendEmail) && userEmail != "" {
		var emailTemplate []byte
		if emailTemplate, err = config.ReadFile("mail.template.html"); err != nil {
			log.Fatalf("Błąd podczas odczytu szablonu email: %v", err)
		}

		mailer := internal.NewMailer(logger)
		// Zaokrąglaj dni do pełnych liczb całkowitych dla czytelności
		daysUntilExpiryInt := int(daysUntilExpiry)
		if err = mailer.SendEmail(ovpnConfig, daysUntilExpiryInt, string(emailTemplate), *commonName, userEmail); err != nil {
			log.Fatalf("Błąd podczas wysyłania e-maila: %v", err)
		}

		if certificateRenewed {
			logger.Infof("Konfiguracja OpenVPN została wysłana na e-mail: %s", userEmail)
		} else {
			logger.Infof("Konfiguracja OpenVPN została ponownie wysłana na e-mail: %s", userEmail)
		}
	} else if !certificateRenewed && !*resendEmail {
		logger.Infof("Nie wysyłano emaila - certyfikat nie został odnowiony (użyj --resend aby wymusić wysłanie)")
	} else {
		logger.Warnf("Nie podano adresu e-mail, konfiguracja nie została wysłana")
	}

	// Zapisz bazę danych
	if err := certDB.Save(); err != nil {
		logger.Warnf("Błąd podczas zapisywania bazy danych: %v", err)
	}

	logger.Infof("Serial number nowego certyfikatu: %s", certInfo.SerialNumber)
	logger.Infof("Informacje o certyfikacie zostały zapisane w bazie danych")
}

// handleServerMode obsługuje tryb serwera
func handleServerMode(certDB *internal.CertificateDB, vaultClient *internal.VaultClient, logger *logrus.Logger, commonName, email, ttl, outputDir, mikrotikIP string, forceRenew, resendEmail bool) {
	// Walidacja parametrów dla trymu serwera
	if mikrotikIP == "" {
		log.Fatalf("W trybie serwera wymagany jest adres IP Mikrotika (parametr -i)")
	}

	// Utwórz menedżer serwera
	serverManager := internal.NewServerManager(certDB, vaultClient, logger)

	var serverCert *internal.ServerCertificate
	var err error
	var daysUntilExpiry float64 = 0
	var serverCertificateRenewed bool = false

	// Sprawdź czy certyfikat serwera istnieje
	serverCert, exists := certDB.GetServerCertificate(commonName)

	if exists {
		logger.Infof("Znaleziono certyfikat serwera dla %s", commonName)

		// Sprawdź ważność certyfikatu serwera
		needsRenewal, daysUntil, err := serverManager.CheckServerCertificateExpiry(commonName, 30)
		if err != nil {
			log.Fatalf("Błąd podczas sprawdzania ważności certyfikatu serwera: %v", err)
		}

		daysUntilExpiry = daysUntil
		logger.Infof("Certyfikat serwera wygasa za %.1f dni", daysUntilExpiry)

		if needsRenewal || forceRenew {
			if forceRenew && !needsRenewal {
				logger.Warnf("Wymuszono odnowienie certyfikatu serwera (opcja --force-renew)")
			} else {
				logger.Warnf("Certyfikat serwera wymaga odnowienia (< 30 dni do wygaśnięcia)")
			}

			// Odnów certyfikat serwera
			serverCert, err = serverManager.RenewServerCertificate(commonName, ttl)
			if err != nil {
				log.Fatalf("Błąd podczas odnawiania certyfikatu serwera: %v", err)
			}

			serverCertificateRenewed = true
			logger.Infof("Certyfikat serwera odnowiony, serial: %s", serverCert.SerialNumber)
		} else {
			logger.Infof("Certyfikat serwera nie wymaga odnowienia")
		}
	} else {
		logger.Infof("Generowanie nowego certyfikatu serwera dla %s", commonName)

		// Generuj nowy certyfikat serwera
		serverCert, err = serverManager.SetupServerCertificate(commonName, ttl)
		if err != nil {
			log.Fatalf("Błąd podczas generowania certyfikatu serwera: %v", err)
		}

		serverCertificateRenewed = true
		logger.Infof("Nowy certyfikat serwera wygenerowany, serial: %s", serverCert.SerialNumber)

		// Oblicz dni do wygaśnięcia dla nowo wygenerowanego certyfikatu
		daysUntilExpiry = time.Until(serverCert.ExpiresAt).Hours() / 24
	}

	// Aktualizuj adres IP Mikrotika w certyfikacie serwera
	if mikrotikIP != "" {
		serverCert.MikrotikIP = mikrotikIP
		err = certDB.AddOrUpdateServerCertificate(*serverCert)
		if err != nil {
			logger.Warnf("Błąd podczas aktualizacji adresu IP Mikrotika: %v", err)
		}

		// Wysyłaj certyfikat na Mikrotika TYLKO jeśli został wygenerowany/odnowiony
		if serverCertificateRenewed {
			// Integracja z API Mikrotika - wysłanie certyfikatu
			if os.Getenv("MIKROTIK_USERNAME") != "" && os.Getenv("MIKROTIK_PASSWORD") != "" {
				// Pobierz dane dostępowe Mikrotika ze zmiennych środowiskowych
				mikrotikUsername := os.Getenv("MIKROTIK_USERNAME")
				mikrotikPassword := os.Getenv("MIKROTIK_PASSWORD")

				// Utwórz klienta integracji Mikrotik
				mikrotikClient, err := internal.NewMikrotikIntegration(mikrotikIP, mikrotikUsername, mikrotikPassword, logger)
				if err != nil {
					logger.Warnf("Nie udało się połączyć z Mikrotikiem: %v", err)
					logger.Warnf("Certyfikat zostanie zaktualizowany ręcznie na routerze")
				} else {
					defer mikrotikClient.Close()

					// Wysłij certyfikat na Mikrotik
					if err := mikrotikClient.UploadCertificateToMikrotik(serverCert); err != nil {
						logger.Warnf("Błąd podczas wysyłania certyfikatu na Mikrotik: %v", err)
					} else {
						logger.Infof("Certyfikat serwera został pomyślnie wysłany na routery Mikrotik")
					}
				}
			} else {
				logger.Warnf("Brak danych dostępowych Mikrotika (MIKROTIK_USERNAME, MIKROTIK_PASSWORD)")
				logger.Infof("Certyfikat serwera wymaga ręcznej aktualizacji na routerze Mikrotik (%s)", mikrotikIP)
			}
		} else {
			logger.Infof("Certyfikat serwera nie wymaga odnowienia - pomijam wysyłkę na Mikrotika")
		}
	}

	// Wysyłanie emaila z certyfikatem serwera
	userEmail := email
	if userEmail == "" {
		// Pobierz email z bazy danych użytkownika
		if userCert, exists := certDB.GetUser(commonName); exists {
			userEmail = userCert.Email
		}
	}

	if userEmail != "" {
		var emailTemplate []byte
		if emailTemplate, err = config.ReadFile("mail.template.html"); err != nil {
			log.Fatalf("Błąd podczas odczytu szablonu email: %v", err)
		}

		mailer := internal.NewMailer(logger)
		// Zaokrąglaj dni do pełnych liczb całkowitych dla czytelności
		daysUntilExpiryInt := int(daysUntilExpiry)
		if err = mailer.SendEmail(serverCert.Certificate, daysUntilExpiryInt, string(emailTemplate), commonName, userEmail); err != nil {
			log.Fatalf("Błąd podczas wysyłania e-maila: %v", err)
		}

		logger.Infof("Certyfikat serwera wysłany na e-mail: %s", userEmail)
	} else {
		logger.Warnf("Nie podano adresu e-mail, certyfikat serwera nie został wysłany")
	}

	// Zapisz bazę danych
	if err := certDB.Save(); err != nil {
		logger.Warnf("Błąd podczas zapisywania bazy danych: %v", err)
	}

	logger.Infof("Konfiguracja serwera zakończona")
}