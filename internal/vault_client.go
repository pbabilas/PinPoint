package internal

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/sirupsen/logrus"
)

type VaultClient struct {
	client     *vault.Client
	logger     *logrus.Logger
	pkiPath    string
	role       string
	serverRole string
}

type CertificateInfo struct {
	Certificate   string
	PrivateKey    string
	CAChain       string
	SerialNumber  string
	ExpiresAt     time.Time
	CommonName    string
}

func NewVaultClient(address, roleID, secretID, pkiPath, role, serverRole string, logger *logrus.Logger) (*VaultClient, error) {
	config := vault.DefaultConfig()
	config.Address = address

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("nie udało się utworzyć klienta Vault: %w", err)
	}

	// Autoryzacja przez AppRole
	logger.Info("Autoryzacja do Vault przez AppRole...")
	data := map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	}

	resp, err := client.Logical().Write("auth/approle/login", data)
	if err != nil {
		return nil, fmt.Errorf("nie udało się zalogować przez AppRole: %w", err)
	}

	if resp == nil || resp.Auth == nil || resp.Auth.ClientToken == "" {
		return nil, fmt.Errorf("nie otrzymano tokena z Vault")
	}

	client.SetToken(resp.Auth.ClientToken)
	logger.Infof("Pomyślnie zalogowano do Vault (token expires in: %ds)", resp.Auth.LeaseDuration)

	return &VaultClient{
		client:     client,
		logger:     logger,
		pkiPath:    pkiPath,
		role:       role,
		serverRole: serverRole,
	}, nil
}

// GetCertificateInfo pobiera informacje o certyfikacie o podanym serial number
func (vc *VaultClient) GetCertificateInfo(serialNumber string) (*CertificateInfo, error) {
	path := fmt.Sprintf("%s/cert/%s", vc.pkiPath, serialNumber)

	secret, err := vc.client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("nie udało się pobrać certyfikatu: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("certyfikat o serial number %s nie został znaleziony", serialNumber)
	}

	certPEM, ok := secret.Data["certificate"].(string)
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format certyfikatu")
	}

	// Parsowanie certyfikatu, aby uzyskać datę wygaśnięcia
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, fmt.Errorf("nie udało się zdekodować certyfikatu PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("nie udało się sparsować certyfikatu: %w", err)
	}

	return &CertificateInfo{
		Certificate:  certPEM,
		SerialNumber: serialNumber,
		ExpiresAt:    cert.NotAfter,
		CommonName:   cert.Subject.CommonName,
	}, nil
}

// IssueCertificate generuje nowy certyfikat w Vault
func (vc *VaultClient) IssueCertificate(commonName string, ttl string) (*CertificateInfo, error) {
	path := fmt.Sprintf("%s/issue/%s", vc.pkiPath, vc.role)

	data := map[string]interface{}{
		"common_name": commonName,
		"ttl":         ttl,
	}

	vc.logger.Infof("Generowanie nowego certyfikatu dla %s w Vault", commonName)

	secret, err := vc.client.Logical().Write(path, data)
	if err != nil {
		return nil, fmt.Errorf("nie udało się wygenerować certyfikatu: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("Vault nie zwrócił danych certyfikatu")
	}

	certificate, ok := secret.Data["certificate"].(string)
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format certyfikatu w odpowiedzi")
	}

	privateKey, ok := secret.Data["private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format klucza prywatnego w odpowiedzi")
	}

	caChain, ok := secret.Data["ca_chain"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format ca_chain w odpowiedzi")
	}

	// Konwersja ca_chain do stringa
	var caChainStr string
	for _, ca := range caChain {
		if caStr, ok := ca.(string); ok {
			caChainStr += caStr + "\n"
		}
	}

	serialNumber, ok := secret.Data["serial_number"].(string)
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format serial_number w odpowiedzi")
	}

	// Parsowanie certyfikatu dla daty wygaśnięcia
	block, _ := pem.Decode([]byte(certificate))
	if block == nil {
		return nil, fmt.Errorf("nie udało się zdekodować certyfikatu PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("nie udało się sparsować certyfikatu: %w", err)
	}

	vc.logger.Infof("Nowy certyfikat wygenerowany: serial=%s, expires=%s", serialNumber, cert.NotAfter.Format("2006-01-02"))

	return &CertificateInfo{
		Certificate:  certificate,
		PrivateKey:   privateKey,
		CAChain:      caChainStr,
		SerialNumber: serialNumber,
		ExpiresAt:    cert.NotAfter,
		CommonName:   commonName,
	}, nil
}

// RevokeCertificate odwołuje certyfikat w Vault
func (vc *VaultClient) RevokeCertificate(serialNumber string) error {
	path := fmt.Sprintf("%s/revoke", vc.pkiPath)

	data := map[string]interface{}{
		"serial_number": serialNumber,
	}

	vc.logger.Infof("Odwoływanie certyfikatu %s w Vault", serialNumber)

	_, err := vc.client.Logical().Write(path, data)
	if err != nil {
		return fmt.Errorf("nie udało się odwołać certyfikatu: %w", err)
	}

	vc.logger.Infof("Certyfikat %s został odwołany", serialNumber)
	return nil
}

// CheckCertificateExpiration sprawdza, czy certyfikat wygasa w ciągu podanej liczby dni
func (vc *VaultClient) CheckCertificateExpiration(certInfo *CertificateInfo, daysThreshold int) (bool, float64) {
	daysUntilExpiry := time.Until(certInfo.ExpiresAt).Hours() / 24
	needsRenewal := daysUntilExpiry < float64(daysThreshold)

	return needsRenewal, daysUntilExpiry
}

// RenewCertificate odwołuje stary certyfikat i generuje nowy
func (vc *VaultClient) RenewCertificate(oldSerialNumber, commonName, ttl string) (*CertificateInfo, error) {
	// Odwołaj stary certyfikat
	if err := vc.RevokeCertificate(oldSerialNumber); err != nil {
		vc.logger.Warnf("Nie udało się odwołać starego certyfikatu: %v", err)
		// Kontynuujemy mimo błędu
	}

	// Wygeneruj nowy certyfikat
	return vc.IssueCertificate(commonName, ttl)
}

// GetCACertificate pobiera certyfikat CA
func (vc *VaultClient) GetCACertificate() (string, error) {
	path := fmt.Sprintf("%s/ca/pem", vc.pkiPath)
	ctx := context.Background()

	secret, err := vc.client.Logical().ReadRawWithContext(ctx, path)
	if err != nil {
		return "", fmt.Errorf("nie udało się pobrać certyfikatu CA: %w", err)
	}
	defer secret.Body.Close()

	caCert, err := io.ReadAll(secret.Body)
	if err != nil {
		return "", fmt.Errorf("nie udało się odczytać certyfikatu CA: %w", err)
	}

	return string(caCert), nil
}

// IssueServerCertificate generuje nowy certyfikat serwera
func (vc *VaultClient) IssueServerCertificate(commonName string, ttl string) (*ServerCertificate, error) {
	path := fmt.Sprintf("%s/issue/%s", vc.pkiPath, vc.serverRole)

	data := map[string]interface{}{
		"common_name": commonName,
		"ttl":         ttl,
	}

	vc.logger.Infof("Generowanie nowego certyfikatu serwera dla %s w Vault", commonName)

	secret, err := vc.client.Logical().Write(path, data)
	if err != nil {
		return nil, fmt.Errorf("nie udało się wygenerować certyfikatu serwera: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("Vault nie zwrócił danych certyfikatu serwera")
	}

	certificate, ok := secret.Data["certificate"].(string)
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format certyfikatu serwera w odpowiedzi")
	}

	privateKey, ok := secret.Data["private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format klucza prywatnego serwera w odpowiedzi")
	}

	caChain, ok := secret.Data["ca_chain"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format ca_chain w odpowiedzi")
	}

	// Konwersja ca_chain do stringa
	var caChainStr string
	for _, ca := range caChain {
		if caStr, ok := ca.(string); ok {
			caChainStr += caStr + "\n"
		}
	}

	serialNumber, ok := secret.Data["serial_number"].(string)
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format numeru seryjnego w odpowiedzi")
	}

	// Parsowanie certyfikatu, aby uzyskać datę wygaśnięcia
	block, _ := pem.Decode([]byte(certificate))
	if block == nil {
		return nil, fmt.Errorf("nie udało się zdekodować certyfikatu serwera PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("nie udało się sparsować certyfikatu serwera: %w", err)
	}

	vc.logger.Infof("Nowy certyfikat serwera wygenerowany: serial=%s, expires=%s", serialNumber, cert.NotAfter.Format("2006-01-02"))

	return &ServerCertificate{
		CommonName:   commonName,
		SerialNumber:  serialNumber,
		Certificate:  certificate,
		PrivateKey:   privateKey,
		IssuingCA:   caChainStr,
		CreatedAt:    time.Now(),
		LastRenewed:  time.Now(),
		ExpiresAt:    cert.NotAfter,
		TTL:          ttl,
	}, nil
}

// GetServerCertificate pobiera informacje o certyfikacie serwera z Vault
func (vc *VaultClient) GetServerCertificate(serialNumber string) (*ServerCertificate, error) {
	path := fmt.Sprintf("%s/cert/%s", vc.pkiPath, serialNumber)

	secret, err := vc.client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("nie udało się pobrać certyfikatu serwera: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("certyfikat serwera o numerze seryjnym %s nie został znaleziony", serialNumber)
	}

	certificate, ok := secret.Data["certificate"].(string)
	if !ok {
		return nil, fmt.Errorf("nieprawidłowy format certyfikatu serwera w odpowiedzi")
	}

	// Parsowanie certyfikatu, aby uzyskać datę wygaśnięcia
	block, _ := pem.Decode([]byte(certificate))
	if block == nil {
		return nil, fmt.Errorf("nie udało się zdekodować certyfikatu serwera PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("nie udało się sparsować certyfikatu serwera: %w", err)
	}

	var issuingCA string
	if caData, ok := secret.Data["issuing_ca"]; ok {
		if caStr, ok := caData.(string); ok {
			issuingCA = caStr
		}
	}

	return &ServerCertificate{
		Certificate:  certificate,
		SerialNumber:  serialNumber,
		CommonName:   cert.Subject.CommonName,
		ExpiresAt:    cert.NotAfter,
		CreatedAt:    time.Now(), // Ta informacja nie jest dostępna w Vault
		LastRenewed:  time.Now(), // Ta informacja nie jest dostępna w Vault
		IssuingCA:   issuingCA,
	}, nil
}
