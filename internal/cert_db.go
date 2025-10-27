package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// UserCertificate przechowuje informacje o certyfikacie użytkownika
type UserCertificate struct {
	CommonName  string    `json:"common_name"`
	SerialNumber string   `json:"serial_number"`
	Email       string    `json:"email,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	LastRenewed time.Time `json:"last_renewed"`
	ExpiresAt   time.Time `json:"expires_at"`
	TTL         string    `json:"ttl"`
}

// ServerCertificate przechowuje informacje o certyfikacie serwera
type ServerCertificate struct {
	CommonName    string    `json:"common_name"`
	SerialNumber  string    `json:"serial_number"`
	Certificate   string    `json:"certificate"`
	PrivateKey    string    `json:"private_key,omitempty"`
	IssuingCA    string    `json:"issuing_ca"`
	CreatedAt     time.Time `json:"created_at"`
	LastRenewed   time.Time `json:"last_renewed"`
	ExpiresAt     time.Time `json:"expires_at"`
	TTL           string    `json:"ttl"`
	MikrotikIP   string    `json:"mikrotik_ip,omitempty"`
}

// CertificateDB reprezentuje bazę danych certyfikatów
type CertificateDB struct {
	Users    map[string]UserCertificate `json:"users"`
	Servers  map[string]ServerCertificate `json:"servers"`
	Metadata struct {
		Version     string    `json:"version"`
		LastUpdated time.Time `json:"last_updated"`
	} `json:"metadata"`
	filePath string
	mutex    sync.RWMutex
	logger   *logrus.Logger
}

// NewCertificateDB tworzy nową instancję bazy danych certyfikatów
func NewCertificateDB(filePath string, logger *logrus.Logger) *CertificateDB {
	db := &CertificateDB{
		Users:    make(map[string]UserCertificate),
		Servers:  make(map[string]ServerCertificate),
		filePath: filePath,
		logger:   logger,
	}
	db.Metadata.Version = "1.0"
	return db
}

// LoadCertificateDB wczytuje bazę danych z pliku
func LoadCertificateDB(filePath string, logger *logrus.Logger) (*CertificateDB, error) {
	db := NewCertificateDB(filePath, logger)

	// Sprawdź czy plik istnieje
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logger.Infof("Plik bazy danych %s nie istnieje, tworzę nową bazę", filePath)
		// Utwórz pustą bazę danych
		db.Metadata.LastUpdated = time.Now()
		if err := db.Save(); err != nil {
			return nil, fmt.Errorf("nie udało się utworzyć nowej bazy danych: %w", err)
		}
		return db, nil
	}

	// Wczytaj istniejący plik
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("nie udało się wczytać pliku bazy danych: %w", err)
	}

	db.mutex.Lock()
	defer db.mutex.Unlock()

	if err := json.Unmarshal(data, db); err != nil {
		return nil, fmt.Errorf("nie udało się sparsować pliku bazy danych: %w", err)
	}

	logger.Infof("Wczytano bazę danych z %s, użytkowników: %d", filePath, len(db.Users))
	return db, nil
}

// Save zapisuje bazę danych do pliku
func (db *CertificateDB) Save() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	// Upewnij się, że katalog istnieje
	dir := filepath.Dir(db.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("nie udało się utworzyć katalogu %s: %w", dir, err)
	}

	// Zaktualizuj metadane
	db.Metadata.LastUpdated = time.Now()

	// Zapisz do pliku tymczasowego, a następnie zmień nazwę (atomiczny zapis)
	tempFile := db.filePath + ".tmp"
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("nie udało się zserializować bazy danych: %w", err)
	}

	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("nie udało się zapisać pliku tymczasowego: %w", err)
	}

	if err := os.Rename(tempFile, db.filePath); err != nil {
		return fmt.Errorf("nie udało się zmienić nazwy pliku: %w", err)
	}

	db.logger.Debugf("Baza danych zapisana do %s", db.filePath)
	return nil
}

// GetUser pobiera informacje o użytkowniku na podstawie common name
func (db *CertificateDB) GetUser(commonName string) (*UserCertificate, bool) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	user, exists := db.Users[commonName]
	if !exists {
		return nil, false
	}
	return &user, true
}

// AddOrUpdateUser dodaje nowego użytkownika lub aktualizuje istniejącego
func (db *CertificateDB) AddOrUpdateUser(userCert UserCertificate) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.Users[userCert.CommonName] = userCert
	db.logger.Infof("Dodano/zaktualizowano użytkownika: %s, serial: %s", userCert.CommonName, userCert.SerialNumber)
	return nil
}

// CheckCertificateExpiry sprawdza czy certyfikat wymaga odnowienia
func (db *CertificateDB) CheckCertificateExpiry(commonName string, daysThreshold int) (bool, float64, error) {
	user, exists := db.GetUser(commonName)
	if !exists {
		return false, 0, fmt.Errorf("użytkownik %s nie istnieje w bazie danych", commonName)
	}

	daysUntilExpiry := time.Until(user.ExpiresAt).Hours() / 24
	needsRenewal := daysUntilExpiry < float64(daysThreshold)

	return needsRenewal, daysUntilExpiry, nil
}

// UpdateCertificateInfo aktualizuje informacje o certyfikacie po odnowieniu
func (db *CertificateDB) UpdateCertificateInfo(commonName, serialNumber string, expiresAt time.Time) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	user, exists := db.Users[commonName]
	if !exists {
		return fmt.Errorf("użytkownik %s nie istnieje w bazie danych", commonName)
	}

	user.SerialNumber = serialNumber
	user.ExpiresAt = expiresAt
	user.LastRenewed = time.Now()
	db.Users[commonName] = user

	db.logger.Infof("Zaktualizowano informacje o certyfikacie dla %s, nowy serial: %s", commonName, serialNumber)
	return nil
}

// GetAllUsers zwraca wszystkich użytkowników z bazy danych
func (db *CertificateDB) GetAllUsers() map[string]UserCertificate {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	// Stwórz kopię, aby uniknąć problemów z współbieżnością
	result := make(map[string]UserCertificate)
	for k, v := range db.Users {
		result[k] = v
	}
	return result
}

// DeleteUser usuwa użytkownika z bazy danych
func (db *CertificateDB) DeleteUser(commonName string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, exists := db.Users[commonName]; !exists {
		return fmt.Errorf("użytkownik %s nie istnieje w bazie danych", commonName)
	}

	delete(db.Users, commonName)
	db.logger.Infof("Usunięto użytkownika: %s", commonName)
	return nil
}

// GetServerCertificate pobiera informacje o certyfikacie serwera
func (db *CertificateDB) GetServerCertificate(commonName string) (*ServerCertificate, bool) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	serverCert, exists := db.Servers[commonName]
	if !exists {
		return nil, false
	}
	return &serverCert, true
}

// AddOrUpdateServerCertificate dodaje lub aktualizuje certyfikat serwera
func (db *CertificateDB) AddOrUpdateServerCertificate(serverCert ServerCertificate) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.Servers[serverCert.CommonName] = serverCert
	db.logger.Infof("Dodano/zaktualizowano certyfikat serwera: %s", serverCert.CommonName)
	return nil
}

// GetAllServers zwraca wszystkie certyfikaty serwera
func (db *CertificateDB) GetAllServers() map[string]ServerCertificate {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	// Stwórz kopię, aby uniknąć problemów z współbieżnością
	result := make(map[string]ServerCertificate)
	for k, v := range db.Servers {
		result[k] = v
	}
	return result
}

// DeleteServerCertificate usuwa certyfikat serwera
func (db *CertificateDB) DeleteServerCertificate(commonName string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, exists := db.Servers[commonName]; !exists {
		return fmt.Errorf("certyfikat serwera %s nie istnieje w bazie danych", commonName)
	}

	delete(db.Servers, commonName)
	db.logger.Infof("Usunięto certyfikat serwera: %s", commonName)
	return nil
}