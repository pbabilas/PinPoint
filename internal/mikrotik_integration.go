package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-routeros/routeros/v3"
	"github.com/sirupsen/logrus"
)

// MikrotikIntegration obsługuje komunikację z routerem Mikrotik
type MikrotikIntegration struct {
	client *routeros.Client
	logger *logrus.Logger
}

// NewMikrotikIntegration tworzy nowy klient integracji z Mikrotikiem
func NewMikrotikIntegration(ip, username, password string, logger *logrus.Logger) (*MikrotikIntegration, error) {
	client, err := routeros.Dial(ip+":8728", username, password)
	if err != nil {
		return nil, fmt.Errorf("nie udało się połączyć z Mikrotikiem: %w", err)
	}

	logger.Infof("Połączono z routerem Mikrotik: %s", ip)

	return &MikrotikIntegration{
		client: client,
		logger: logger,
	}, nil
}

// UpdateServerCertificate aktualizuje certyfikat serwera na routerze Mikrotik
func (mi *MikrotikIntegration) UpdateServerCertificate(certName, certPEM, keyPEM string) error {
	defer mi.client.Close()

	mi.logger.Infof("Aktualizowanie certyfikatu serwera na Mikrotiku: %s", certName)

	// Sprawdź czy certyfikat już istnieje
	reply, err := mi.client.Run("/certificate/print", "?name="+certName)
	if err != nil {
		return fmt.Errorf("błąd podczas sprawdzania certyfikatu: %w", err)
	}

	certExists := len(reply.Re) > 0

	if certExists {
		mi.logger.Infof("Certyfikat %s już istnieje, usuwanie starego", certName)
		// Usuń stary certyfikat
		if err := mi.revokeCertificate(certName); err != nil {
			mi.logger.Warnf("Nie udało się usunąć starego certyfikatu: %v", err)
			// Kontynuujemy mimo błędu
		}
	}

	// Importuj nowy certyfikat
	if err := mi.importCertificate(certName, certPEM, keyPEM); err != nil {
		return fmt.Errorf("nie udało się zaimportować certyfikatu: %w", err)
	}

	mi.logger.Infof("Certyfikat %s został pomyślnie zaktualizowany na Mikrotiku", certName)
	return nil
}

// revokeCertificate usuwa certyfikat z routera
func (mi *MikrotikIntegration) revokeCertificate(certName string) error {
	// Pobierz ID certyfikatu
	reply, err := mi.client.Run("/certificate/print", "?name="+certName)
	if err != nil {
		return fmt.Errorf("błąd podczas pobierania certyfikatu: %w", err)
	}

	if len(reply.Re) == 0 {
		return fmt.Errorf("certyfikat %s nie został znaleziony", certName)
	}

	certID := reply.Re[0].Map[".id"]

	// Usuń certyfikat
	_, err = mi.client.Run("/certificate/remove", "=.id="+certID)
	if err != nil {
		return fmt.Errorf("błąd podczas usuwania certyfikatu: %w", err)
	}

	mi.logger.Infof("Certyfikat %s został usunięty", certName)
	return nil
}

// importCertificate importuje nowy certyfikat do routera
func (mi *MikrotikIntegration) importCertificate(certName, certPEM, keyPEM string) error {
	// Tworzymy unikalną nazwę pliku
	timestamp := time.Now().Format("20060102150405")
	certFileName := fmt.Sprintf("/tmp/cert_%s_%s.pem", certName, timestamp)
	keyFileName := fmt.Sprintf("/tmp/key_%s_%s.pem", certName, timestamp)

	// Importujemy certyfikat
	_, err := mi.client.Run(
		"/certificate/import",
		"=file-name="+certFileName,
		"=key-file-name="+keyFileName,
		"=passphrase=",
		"=name="+certName,
	)

	if err != nil {
		return fmt.Errorf("błąd podczas importu certyfikatu: %w", err)
	}

	mi.logger.Infof("Certyfikat %s został zaimportowany", certName)

	// Czyść pliki tymczasowe
	mi.cleanupTempFiles(certFileName, keyFileName)

	return nil
}

// cleanupTempFiles usuwa pliki tymczasowe z routera
func (mi *MikrotikIntegration) cleanupTempFiles(certFile, keyFile string) {
	mi.logger.Debugf("Czyszczenie plików tymczasowych")

	// Utwórz listę plików do usunięcia
	filesToDelete := []string{certFile, keyFile}

	for _, file := range filesToDelete {
		_, err := mi.client.Run("/file/remove", "=.id="+file)
		if err != nil {
			mi.logger.Warnf("Nie udało się usunąć pliku tymczasowego %s: %v", file, err)
		}
	}
}

// UploadCertificateToMikrotik wysyła certyfikat na router Mikrotik
func (mi *MikrotikIntegration) UploadCertificateToMikrotik(serverCert *ServerCertificate) error {
	if serverCert.MikrotikIP == "" {
		return fmt.Errorf("adres IP Mikrotika nie został skonfigurowany")
	}

	// Uzupełniaj nazwę certyfikatu, jeśli jest pusta
	certName := serverCert.CommonName
	if certName == "" {
		certName = "ovpn-server-" + serverCert.SerialNumber[:8]
	}

	// Aktualizuj certyfikat na routerze
	if err := mi.UpdateServerCertificate(certName, serverCert.Certificate, serverCert.PrivateKey); err != nil {
		return fmt.Errorf("błąd podczas aktualizacji certyfikatu na Mikrotiku: %w", err)
	}

	mi.logger.Infof("Certyfikat został pomyślnie wysłany na Mikrotik: %s (%s)", certName, serverCert.MikrotikIP)
	return nil
}

// GetCertificateStatus pobiera status certyfikatu z routera
func (mi *MikrotikIntegration) GetCertificateStatus(certName string) (map[string]string, error) {
	reply, err := mi.client.Run("/certificate/print", "?name="+certName)
	if err != nil {
		return nil, fmt.Errorf("błąd podczas pobierania statusu certyfikatu: %w", err)
	}

	if len(reply.Re) == 0 {
		return nil, fmt.Errorf("certyfikat %s nie został znaleziony", certName)
	}

	return reply.Re[0].Map, nil
}

// ListCertificates listuje wszystkie certyfikaty na routerze
func (mi *MikrotikIntegration) ListCertificates() ([]map[string]string, error) {
	reply, err := mi.client.Run("/certificate/print")
	if err != nil {
		return nil, fmt.Errorf("błąd podczas listy certyfikatów: %w", err)
	}

	var certs []map[string]string
	for _, re := range reply.Re {
		certs = append(certs, re.Map)
	}

	return certs, nil
}

// ValidateCertificate sprawdza, czy certyfikat jest poprawny i ważny
func (mi *MikrotikIntegration) ValidateCertificate(certName string) (bool, error) {
	status, err := mi.GetCertificateStatus(certName)
	if err != nil {
		return false, err
	}

	// Sprawdzamy czy certyfikat ma invalid=false
	invalid, ok := status["invalid"]
	if !ok || strings.ToLower(invalid) == "true" {
		return false, nil
	}

	return true, nil
}
