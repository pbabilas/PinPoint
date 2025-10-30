package internal

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/go-routeros/routeros/v3"
	"github.com/jlaffaye/ftp"
	"github.com/sirupsen/logrus"
)

// MikrotikIntegration obsługuje komunikację z routerem Mikrotik
type MikrotikIntegration struct {
	client   *routeros.Client
	ip       string
	username string
	password string
	logger   *logrus.Logger
}

// NewMikrotikIntegration tworzy nowy klient integracji z Mikrotikiem
func NewMikrotikIntegration(ip, username, password string, logger *logrus.Logger) (*MikrotikIntegration, error) {
	// Połączenie RouterOS API
	client, err := routeros.Dial(ip+":8728", username, password)
	if err != nil {
		return nil, fmt.Errorf("nie udało się połączyć z Mikrotikiem (API): %w", err)
	}

	// Test połączenia FTP
	ftpClient, err := ftp.Dial(ip + ":21")
	if err == nil {
		defer ftpClient.Quit()
		err = ftpClient.Login(username, password)
		if err == nil {
			logger.Infof("Połączono FTP z routerem Mikrotik: %s", ip)
		} else {
			logger.Warnf("Nie udało się zalogować FTP: %v - certyfikat będzie musiał być zaimportowany ręcznie", err)
		}
	} else {
		logger.Warnf("Nie udało się połączyć FTP: %v - certyfikat będzie musiał być zaimportowany ręcznie", err)
	}

	logger.Infof("Połączono z routerem Mikrotik: %s", ip)

	return &MikrotikIntegration{
		client:   client,
		ip:       ip,
		username: username,
		password: password,
		logger:   logger,
	}, nil
}

// UpdateServerCertificate aktualizuje certyfikat serwera na routerze Mikrotik
func (mi *MikrotikIntegration) UpdateServerCertificate(certName, certPEM, keyPEM string) error {
	mi.logger.Infof("Aktualizowanie certyfikatu serwera na Mikrotiku: %s", certName)

	// Próbuj usunąć stary certyfikat jeśli istnieje
	// Ignoruj błędy - certyfikat może nie istnieć
	_ = mi.revokeCertificate(certName)

	// Importuj nowy certyfikat
	if err := mi.importCertificate(certName, certPEM, keyPEM); err != nil {
		return fmt.Errorf("nie udało się zaimportować certyfikatu: %w", err)
	}

	mi.logger.Infof("Certyfikat %s został pomyślnie zaktualizowany na Mikrotiku", certName)

	// Skonfiguruj serwer OpenVPN aby używał tego certyfikatu
	if err := mi.configureOpenVPNServer(certName); err != nil {
		mi.logger.Warnf("Nie udało się skonfigurować serwera OpenVPN: %v", err)
		// Kontynuujemy mimo błędu - cert jest zaimportowany
	}

	return nil
}

// configureOpenVPNServer ustawia certyfikat w konfiguracji serwera OpenVPN
func (mi *MikrotikIntegration) configureOpenVPNServer(certName string) error {
	mi.logger.Infof("Konfigurowanie serwera OpenVPN do użycia certyfikatu: %s", certName)

	// Najpierw sprawdź aktualną konfigurację
	resp, err := mi.client.Run("/interface/ovpn-server/server/print")
	if err != nil {
		mi.logger.Warnf("Nie udało się odczytać konfiguracji OpenVPN: %v", err)
	} else if len(resp.Re) > 0 {
		mi.logger.Infof("Aktualna konfiguracja OpenVPN: %v", resp.Re[0].Map)
	}

	// Ustaw certyfikat w `/interface/ovpn-server/server`
	// RouterOS wymaga numbers=0 aby edytować pierwszy (zazwyczaj jedyny) element
	resp, err = mi.client.Run(
		"/interface/ovpn-server/server/set",
		"=numbers=0",
		"=certificate="+certName,
	)

	if err != nil {
		return fmt.Errorf("błąd podczas konfiguracji serwera OpenVPN: %w", err)
	}

	mi.logger.Infof("Polecenie set certificate zwróciło: %v", resp)

	// Pobierz ID konfiguracji
	resp, err = mi.client.Run("/interface/ovpn-server/server/print")
	if err == nil && len(resp.Re) > 0 {
		configID := resp.Re[0].Map[".id"]
		mi.logger.Infof("ID konfiguracji OpenVPN: %s", configID)
		mi.logger.Infof("Nowa konfiguracja OpenVPN: %v", resp.Re[0].Map)

		// Sprawdzamy czy certyfikat się zmienił
		if cert, ok := resp.Re[0].Map["certificate"]; ok {
			if cert == certName {
				mi.logger.Infof("✓ Certyfikat pomyślnie zmieniony na: %s", certName)
			} else {
				mi.logger.Warnf("Certyfikat nie zmienił się! Wciąż: %s", cert)
			}
		}
	}

	mi.logger.Infof("Serwer OpenVPN skonfigurowany do użycia certyfikatu: %s", certName)
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

// uploadFileViaFTP wysyła plik na router przez FTP
func (mi *MikrotikIntegration) uploadFileViaFTP(remoteFilename, fileContent string) error {
	// Połącz FTP
	ftpClient, err := ftp.Dial(mi.ip + ":21")
	if err != nil {
		return fmt.Errorf("nie udało się połączyć FTP: %w", err)
	}
	defer ftpClient.Quit()

	// Zaloguj się
	err = ftpClient.Login(mi.username, mi.password)
	if err != nil {
		return fmt.Errorf("nie udało się zalogować FTP: %w", err)
	}

	// Wyślij plik
	err = ftpClient.Stor(remoteFilename, bytes.NewReader([]byte(fileContent)))
	if err != nil {
		return fmt.Errorf("błąd podczas wysyłania pliku FTP: %w", err)
	}

	mi.logger.Infof("Plik wysłany na router FTP: %s", remoteFilename)
	return nil
}

// importCertificate importuje nowy certyfikat do routera
func (mi *MikrotikIntegration) importCertificate(certName, certPEM, keyPEM string) error {
	// Tworzymy unikalną nazwę pliku - RouterOS potrzebuje jednego pliku z cert+key
	timestamp := time.Now().Format("20060102150405")
	certFileName := fmt.Sprintf("flash/%s_%s.pem", certName, timestamp)

	// Scalamy cert i key w jeden plik
	combinedPEM := certPEM + "\n" + keyPEM

	// Ścieżka dla FTP (z slashem na początku)
	ftpPath := "/" + certFileName
	mi.logger.Infof("Wysyłanie certyfikatu (cert+key) do %s", ftpPath)
	if err := mi.uploadFileViaFTP(ftpPath, combinedPEM); err != nil {
		mi.logger.Warnf("Błąd przy wysyłaniu certyfikatu przez FTP: %v", err)
		return fmt.Errorf("nie udało się wysłać certyfikatu na router: %w", err)
	}

	mi.logger.Infof("Certyfikat wysłany na router przez FTP - importowanie")

	// Importujemy certyfikat
	_, err := mi.client.Run(
		"/certificate/import",
		"=file-name="+certFileName,
		"=name="+certName,
	)

	if err != nil {
		return fmt.Errorf("błąd podczas importu certyfikatu: %w", err)
	}

	mi.logger.Infof("Certyfikat %s został zaimportowany", certName)

	// Czyść plik tymczasowy
	mi.cleanupTempFile(certFileName)

	return nil
}

// cleanupTempFile usuwa plik tymczasowy z routera
func (mi *MikrotikIntegration) cleanupTempFile(file string) {
	mi.logger.Debugf("Czyszczenie pliku tymczasowego: %s", file)

	_, err := mi.client.Run("/file/remove", "=.id="+file)
	if err != nil {
		mi.logger.Warnf("Nie udało się usunąć pliku tymczasowego %s: %v", file, err)
	}
}

// cleanupTempFiles usuwa pliki tymczasowe z routera (deprecated - używaj cleanupTempFile)
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

	// Wysyłamy tylko certificate + private key (bez łańcucha CA)
	// RouterOS będzie potrzebować CA chain zaimportowany osobno jeśli go nie ma
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

// Close zamyka połączenia z routerem
func (mi *MikrotikIntegration) Close() error {
	if mi.client != nil {
		mi.client.Close()
	}
	return nil
}
