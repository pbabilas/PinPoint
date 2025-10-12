package main

import (
	"b-code.cloud/routeros/ovpn/internal"
	"embed"
	"fmt"
	"github.com/akamensky/argparse"
	"github.com/go-routeros/routeros/v3"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"log"
	"os"
)

var config embed.FS

func main() {

	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}
	parser := argparse.NewParser("routeros-util", "Configure wasabi for project")
	address := parser.String("a", "address", &argparse.Options{Required: false, Help: "Mikrotik address", Default: "192.168.1.1:8728"})
	certName := parser.String("n", "name", &argparse.Options{Required: false, Help: "Certificate name", Default: "ovpn-pbabilas-040524"})
	passphrase := parser.String("p", "passphrase", &argparse.Options{Required: true, Help: "Certificate passphrase", Default: "myStrongPassword"})

	logger := &logrus.Logger{
		Out:          os.Stderr,
		Formatter:    new(logrus.TextFormatter),
		Hooks:        make(logrus.LevelHooks),
		Level:        logrus.InfoLevel,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}
	if err = parser.Parse(os.Args); err != nil {
		logger.Fatalf(parser.Usage(err))
	}

	var client *routeros.Client
	if client, err = routeros.Dial(*address, os.Getenv("MIKROTIK_USERNAME"), os.Getenv("MIKROTIK_PASSWORD")); err != nil {
		log.Fatalf("Error connecting to routeros: %v", err)
	}
	defer client.Close()

	routerOs := internal.NewRouterOS(client)
	certManager := internal.NewCertManager(*routerOs, logger)
	logger.Infof("Started ovpn certificate validate!")
	certVal := certManager.GetCert(*routerOs, *certName)
	expireAfter := certVal["expires-after"]
	daysExpire, err := certManager.ParseDuration(expireAfter)

	if daysExpire < 30 {
		logger.Warnf("Certificate expires after %f days, renewing", daysExpire)
		if err = certManager.RenewCert(*routerOs, certVal); err != nil {
			log.Fatalf("Błąd podczas odnowywania certyfikatu: %v", err)
		}

		exportPassphrase := passphrase // Hasło do eksportu certyfikatu
		clientCertFileName := certName // Nazwa pliku z certyfikatem PEM

		_, err = routerOs.Cmd([]string{
			"/certificate/export-certificate",
			"=.id=" + *certName,
			"=type=pem",
			"=file-name=" + *clientCertFileName,
			"=export-passphrase=" + *exportPassphrase,
		})
		if err != nil {
			log.Fatalf("Błąd podczas eksportu certyfikatu: %v", err)
		}

		logger.Infof("Certificate %s exported to %s.(pem,key)", *certName, *clientCertFileName)

		crt := certManager.ReadCert(client, *clientCertFileName+".crt")
		key := certManager.ReadCert(client, *clientCertFileName+".key")
		var ovpnConfig []byte
		if ovpnConfig, err = config.ReadFile("user.ovpn.template"); err != nil {
			log.Fatalf("Błąd podczas odczytu pliku: %v", err)
		}
		config := fmt.Sprintf(string(ovpnConfig), crt, key)
		mailer := internal.NewMailer(logger)
		if err = mailer.SendEmail(config, daysExpire); err != nil {
			log.Fatalf("Błąd podczas wysyłania e-maila: %v", err)
		}
	} else {
		log.Printf("Certificate expires after %f days, not renewing", daysExpire)
	}
}
