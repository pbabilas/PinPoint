package internal

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// ServerManager zarządza certyfikatami serwera OpenVPN
type ServerManager struct {
	certDB  *CertificateDB
	vaultClient *VaultClient
	logger    *logrus.Logger
}

// NewServerManager tworzy nowy menedżer serwera
func NewServerManager(certDB *CertificateDB, vaultClient *VaultClient, logger *logrus.Logger) *ServerManager {
	return &ServerManager{
		certDB:     certDB,
		vaultClient: vaultClient,
		logger:      logger,
	}
}

// SetupServerCertificate konfiguruje nowy certyfikat serwera
func (sm *ServerManager) SetupServerCertificate(commonName string, ttl string) (*ServerCertificate, error) {
	sm.logger.Infof("Konfiguracja nowego certyfikatu serwera dla %s", commonName)

	// Sprawdź czy certyfikat serwera już istnieje
	serverCert, exists := sm.certDB.GetServerCertificate(commonName)
	if exists {
		sm.logger.Infof("Certyfikat serwera dla %s już istnieje", commonName)
		return serverCert, nil
	}

	// Generuj nowy certyfikat serwera
	newServerCert, err := sm.vaultClient.IssueServerCertificate(commonName, ttl)
	if err != nil {
		return nil, fmt.Errorf("błąd podczas generowania certyfikatu serwera: %w", err)
	}

	// Zapisz certyfikat serwera w bazie danych
	err = sm.certDB.AddOrUpdateServerCertificate(*newServerCert)
	if err != nil {
		sm.logger.Warnf("Błąd podczas zapisywania certyfikatu serwera w bazie danych: %v", err)
	}

	sm.logger.Infof("Nowy certyfikat serwera wygenerowany: serial=%s", newServerCert.SerialNumber)
	return newServerCert, nil
}

// RenewServerCertificate odnawia certyfikat serwera
func (sm *ServerManager) RenewServerCertificate(commonName string, ttl string) (*ServerCertificate, error) {
	sm.logger.Infof("Odnawianie certyfikatu serwera dla %s", commonName)

	// Pobierz istniejący certyfikat serwera
	_, exists := sm.certDB.GetServerCertificate(commonName)
	if !exists {
		return nil, fmt.Errorf("certyfikat serwera dla %s nie istnieje", commonName)
	}

	// Tutaj byłaby logika odnawiania certyfikatu serwera, ale na razie generujemy nowy
	// W przyszłości można zaimplementować prawdziwe odnawianie z zachowaniem numeru seryjnego
	sm.logger.Warnf("Generowanie nowego certyfikatu serwera zamiast odnawiania (do zaimplementowania w przyszłości)")

	// Generuj nowy certyfikat serwera
	newServerCert, err := sm.vaultClient.IssueServerCertificate(commonName, ttl)
	if err != nil {
		return nil, fmt.Errorf("błąd podczas generowania nowego certyfikatu serwera: %w", err)
	}

	// Zapisz nowy certyfikat serwera w bazie danych
	err = sm.certDB.AddOrUpdateServerCertificate(*newServerCert)
	if err != nil {
		sm.logger.Warnf("Błąd podczas zapisywania certyfikatu serwera w bazie danych: %v", err)
	}

	sm.logger.Infof("Nowy certyfikat serwera wygenerowany: serial=%s", newServerCert.SerialNumber)
	return newServerCert, nil
}

// GetServerCertificate pobiera informacje o certyfikacie serwera
func (sm *ServerManager) GetServerCertificate(commonName string) (*ServerCertificate, bool) {
	return sm.certDB.GetServerCertificate(commonName)
}

// CheckServerCertificateExpiry sprawdza ważność certyfikatu serwera
func (sm *ServerManager) CheckServerCertificateExpiry(commonName string, daysThreshold int) (bool, float64, error) {
	serverCert, exists := sm.certDB.GetServerCertificate(commonName)
	if !exists {
		return false, 0, fmt.Errorf("certyfikat serwera dla %s nie istnieje", commonName)
	}

	daysUntilExpiry := time.Until(serverCert.ExpiresAt).Hours() / 24
	needsRenewal := daysUntilExpiry < float64(daysThreshold)

	return needsRenewal, daysUntilExpiry, nil
}

// GetAllServers zwraca wszystkie certyfikaty serwera
func (sm *ServerManager) GetAllServers() map[string]ServerCertificate {
	return sm.certDB.GetAllServers()
}

// DeleteServerCertificate usuwa certyfikat serwera
func (sm *ServerManager) DeleteServerCertificate(commonName string) error {
	return sm.certDB.DeleteServerCertificate(commonName)
}