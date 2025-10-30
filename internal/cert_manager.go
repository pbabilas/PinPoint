package internal

import (
	"fmt"
	"github.com/go-routeros/routeros/v3"
	"github.com/sirupsen/logrus"
	"log"
	"regexp"
	"strconv"
	"time"
)

type CertManager struct {
	client RouterOS
	logger *logrus.Logger
}

func NewCertManager(client RouterOS, logger *logrus.Logger) *CertManager {
	return &CertManager{client: client, logger: logger}
}

func (cm *CertManager) ParseDuration(input string) (float64, error) {
	// Regex do dopasowania wartości tygodni, dni, godzin, minut i sekund (wszystkie są opcjonalne)
	re := regexp.MustCompile(`(?P<weeks>\d+w)?(?P<days>\d+d)?(?P<hours>\d+h)?(?P<minutes>\d+m)?(?P<seconds>\d+s)?`)

	matches := re.FindStringSubmatch(input)
	if matches == nil {
		return 0, fmt.Errorf("Niepoprawny format")
	}

	var totalDays float64

	// Funkcja pomocnicza do przekształcania wartości na godziny
	addDuration := func(value string, daysPerUnit float64) {
		if value != "" {
			number, _ := strconv.Atoi(value[:len(value)-1]) // Usuwa jednostkę (np. 'w', 'd', etc.)
			totalDays += float64(number) * daysPerUnit
		}
	}

	// Parsowanie poszczególnych jednostek i przeliczanie na godziny
	addDuration(matches[1], 7) // tygodnie na godziny
	addDuration(matches[2], 1) // dni na godziny

	return totalDays, nil
}

func (cm *CertManager) RenewCert(routerOs RouterOS, certVal map[string]string) (err error) {
	certName := certVal["name"]
	if _, err := routerOs.Cmd([]string{"/certificate/issued-revoke", "=numbers=" + certName}); err != nil {
		log.Fatalf("Błąd podczas wykonywania komendy: %v", err)
	}
	systemDate := time.Now().Format("2006-01-02")
	newCertName := certName + "-revoked-" + systemDate
	_, err = routerOs.Cmd([]string{"/certificate/set", "=name=" + newCertName, "=.id=" + certName})
	if err != nil {
		return err
	}
	cm.logger.Infof("Certificate %s revoked with name %s", certName, newCertName)

	commonName := certVal["common-name"]
	keyUsage := certVal["key-usage"]
	subjectAltName := certVal["subject-alt-name"]

	_, err = routerOs.Cmd([]string{
		"/certificate/add",
		"=name=" + certName,
		"=common-name=" + commonName,
		"=key-usage=" + keyUsage,
		"=subject-alt-name=" + subjectAltName,
	})
	if err != nil {
		return err
	}
	cm.logger.Infof("New certificate %s created", certName)

	caCert := certVal["ca"]

	if certName == "" || caCert == "" {
		return fmt.Errorf("Nie udało się pobrać wymaganych danych z certyfikatu.")
	}

	_, err = routerOs.Cmd([]string{
		"/certificate/sign",
		"=name=" + certName,
		"=ca=" + caCert,
		"=.id=" + certName,
	})
	if err != nil {
		log.Fatalf("Błąd podczas podpisywania certyfikatu: %v", err)
	}
	cm.logger.Infof("Certificate %s was signed with CA", certName)

	return nil
}

func (cm *CertManager) GetCert(client RouterOS, certName string) map[string]string {
	res, err := client.Cmd([]string{"/certificate/print", "?name=" + certName})
	if err != nil {
		log.Fatalf("Błąd podczas wykonywania komendy: %v", err)
	}
	return res[0]
}

func (cm *CertManager) ReadCert(client *routeros.Client, fileName string) string {
	cm.logger.Infof("Reading cert file %s", fileName)
	reply, err := client.Run("/file/print", "?name="+fileName)
	if err != nil {
		log.Fatalf("Błąd podczas pobierania szczegółów pliku: %v", err)
	}

	if len(reply.Re) == 0 {
		log.Fatalf("Plik %s nie został znaleziony", fileName)
	}

	fileSize, err := strconv.Atoi(reply.Re[0].Map["size"])
	if err != nil {
		log.Fatalf("Błąd podczas konwersji rozmiaru pliku: %v", err)
	}

	chunkSize := fileSize // Możesz dostosować chunk-size w zależności od potrzeb
	chunkReply, err := client.Run("/file/read", "=file="+fileName, "=chunk-size="+strconv.Itoa(chunkSize))
	if err != nil {
		log.Fatalf("Błąd podczas odczytu pliku: %v", err)
	}

	return chunkReply.Re[0].Map["data"]
}
