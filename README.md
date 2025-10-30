# PinPoint

Narzędzie do precyzyjnego zarządzania certyfikatami OpenVPN z integracją HashiCorp Vault i Mikrotik RouterOS.

**English**: Tool for precise OpenVPN certificate management with HashiCorp Vault and Mikrotik RouterOS integration.

## Funkcionalność / Features

- 🔐 **Integracja z Vault** - Automatyczne wydawanie i odnawianie certyfikatów z HashiCorp Vault PKI
- 📧 **Email Notifications** - Wysyłanie konfiguracji OpenVPN drogą mailową
- 🔄 **Automatyczne Odnawianie** - Automatyczne odnawianie certyfikatów przed wygaśnięciem (30 dni przed datą)
- 🌐 **Mikrotik Integration** - Automatyczne wdrażanie certyfikatów serwera na urządzeniach Mikrotik
- 💾 **Baza Danych** - Trwała baza danych certyfikatów w formacie JSON
- 🖥️ **Dwa Tryby** - Obsługa certyfikatów klienta (client mode) i serwera (server mode)

## Wymagania / Requirements

- Go 1.16+
- HashiCorp Vault (z konfiguracją PKI)
- Dostęp do Vault API
- Dla trybu serwera: dostęp SSH do routerów Mikrotik
- Dla email: konfiguracja SMTP

## Instalacja / Installation

### Z kodu źródłowego / From Source

```bash
git clone https://github.com/your-repo/pinpoint.git
cd pinpoint
make build
```

Skompilowany plik znajduje się w: `bin/pinpoint`

Dla innych systemów operacyjnych:
```bash
# macOS
GOOS=darwin GOARCH=amd64 make build

# Windows
GOOS=windows GOARCH=amd64 make build
```

## Konfiguracja / Configuration

### 1. HashiCorp Vault - Konfiguracja PKI

#### 1.1 Włączenie PKI Secrets Engine

```bash
# Zaloguj się do Vault
vault login

# Włącz PKI secrets engine
vault secrets enable pki

# Ustaw maksymalny TTL (np. 87600h = 10 lat)
vault secrets tune -max-lease-ttl=87600h pki
```

#### 1.2 Generowanie Root CA

```bash
# Wygeneruj root certificate
vault write -field=certificate pki/root/generate/internal \
  common_name="OpenVPN Root CA" \
  ttl=87600h > root_ca.crt

# Skonfiguruj CDP i AIA
vault write pki/config/urls \
  issuing_certificates="http://vault.example.com/v1/pki/ca" \
  crl_distribution_points="http://vault.example.com/v1/pki/crl"
```

#### 1.3 Tworzenie Roli dla Certyfikatów Klienta

```bash
# Utwórz rolę dla klientów VPN
vault write pki/roles/ovpn-client \
  allowed_domains="client.vpn" \
  allow_subdomains=true \
  max_ttl="8760h" \
  generate_lease=true \
  key_type="rsa" \
  key_bits=2048
```

#### 1.4 Tworzenie Roli dla Certyfikatów Serwera

```bash
# Utwórz rolę dla serwerów VPN
vault write pki/roles/ovpn-server \
  allowed_domains="vpn.example.com" \
  allow_subdomains=true \
  max_ttl="8760h" \
  generate_lease=true \
  key_type="rsa" \
  key_bits=2048
```

#### 1.5 Konfiguracja AppRole

AppRole jest używana do uwierzytelniania się narzędzia z Vault bez interaktywnego logowania.

```bash
# Włącz AppRole auth method
vault auth enable approle

# Utwórz politykę dla naszego narzędzia
cat > /tmp/ovpn-policy.hcl << EOF
path "pki/issue/*" {
  capabilities = ["create", "update"]
}
path "pki/sign/*" {
  capabilities = ["create", "update"]
}
path "secret/data/ovpn/*" {
  capabilities = ["read", "list"]
}
EOF

vault policy write ovpn-policy /tmp/ovpn-policy.hcl

# Utwórz AppRole
vault write auth/approle/role/ovpn-cert-renew \
  token_num_uses=0 \
  token_ttl=1h \
  token_max_ttl=4h \
  policies="ovpn-policy"

# Otrzymaj Role ID
vault read auth/approle/role/ovpn-cert-renew/role-id

# Wygeneruj Secret ID (ważny przez 24 godziny)
vault write -f auth/approle/role/ovpn-cert-renew/secret-id
```

### 2. Zmienne Środowiskowe / Environment Variables

Utwórz plik `.env` w głównym katalogu projektu:

```bash
# Vault Configuration
VAULT_ADDR=https://vault.example.com:8200
VAULT_ROLE_ID=your-role-id-here
VAULT_SECRET_ID=your-secret-id-here
VAULT_PKI_PATH=pki
VAULT_ROLE=ovpn-client

# Server Mode (opcjonalnie)
VAULT_SERVER_ROLE=ovpn-server

# Mikrotik Configuration (dla trybu serwera)
MIKROTIK_USERNAME=admin
MIKROTIK_PASSWORD=your-mikrotik-password

# SMTP Configuration (dla emaili)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-app-password
SMTP_FROM=vpn-admin@example.com
SMTP_TLS=true
```

**⚠️ WAŻNE**: Nigdy nie commituj `.env` do repozytorium! Dodaj do `.gitignore`:

```bash
echo ".env" >> .gitignore
```

## Konfiguracja Mikrotik / Mikrotik Setup

### 1. Przygotowanie Routera Mikrotik

#### 1.1 Włączenie SSH

```bash
# W WebFig lub poprzez Winbox:
# IP → Services → SSH
# Ustaw port SSH (domyślnie 22)
# Włącz usługę SSH
```

Lub przez SSH (jeśli już jest dostępny):
```bash
/ip service set ssh disabled=no port=22
```

#### 1.2 Tworzenie Użytkownika do Zarządzania Certyfikatami

```bash
# Zaloguj się do routera SSH
ssh admin@192.168.1.1

# Utwórz nowego użytkownika
/user add name=vpn-admin password=secure-password group=full
```

#### 1.3 Konfiguracja OpenVPN na Routeriku

```bash
# Wejdź do konfiguracji OpenVPN
/interface/ovpn-server/server

# Ustaw parametry:
set enabled=yes
set certificate="server-cert" (będzie wstawiony przez narzędzie)
set protocol=tcp
set port=1194
set require-client-certificate=yes
```

### 2. Przygotowanie Certyfikatów

Narzędzie automatycznie:
1. Generuje certyfikaty w Vault
2. Pobiera je jako plik
3. Wysyła do Mikrotika poprzez SSH
4. Konfiguruje je w OpenVPN

Wymagane jest tylko podanie adresu IP Mikrotika i danych dostępu.

## Użycie / Usage

### Tryb Klienta / Client Mode

#### Wygenerowanie Certyfikatu Klienta

```bash
# Podstawowe użycie
./bin/pinpoint -n pbabilas.client.vpn

# Z niestandardowym emailem
./bin/pinpoint -n pbabilas.client.vpn -e pbabilas@example.com

# Z niestandardowym TTL (Time To Live)
./bin/pinpoint -n pbabilas.client.vpn -t 8760h

# Z niestandardowym katalogiem wyjściowym
./bin/pinpoint -n pbabilas.client.vpn -o ./my-configs
```

#### Wymuszenie Odnowienia

```bash
# Nawet jeśli certyfikat nie wygasł
./bin/pinpoint -n pbabilas.client.vpn --force-renew
```

#### Wysłanie Maila Ponownie

```bash
# Bez regenerowania certyfikatu
./bin/pinpoint -n pbabilas.client.vpn --resend
```

### Tryb Serwera / Server Mode

#### Konfiguracja Certyfikatu Serwera

```bash
# Podstawowe użycie
./bin/pinpoint -m server \
  -n vpn.example.com \
  -i 192.168.1.1 \
  -e admin@example.com

# Wyjaśnienie parametrów:
# -m server        = tryb serwera
# -n vpn.example.com = common name (CN) dla certyfikatu serwera
# -i 192.168.1.1   = adres IP routera Mikrotik
# -e admin@...     = email do powiadomienia
```

#### Wymuszenie Odnowienia Serwera

```bash
./bin/pinpoint -m server \
  -n vpn.example.com \
  -i 192.168.1.1 \
  --force-renew
```

## Flagi Wiersza Poleceń / Command Line Flags

| Flaga | Długa forma | Opis | Domyślne |
|-------|-------------|------|---------|
| `-n` | `--name` | Common Name certyfikatu (wymagane) | `ovpn-pbabilas` |
| `-e` | `--email` | Email do powiadomień | (brak) |
| `-t` | `--ttl` | TTL certyfikatu | `8760h` |
| `-o` | `--output-dir` | Katalog dla plików .ovpn | `conf` |
| `-d` | `--cert-db` | Ścieżka do bazy certyfikatów | `certificates.json` |
| `-f` | `--force-renew` | Wymuszenie odnowienia | `false` |
| `-r` | `--resend` | Ponowne wysłanie maila | `false` |
| `-m` | `--mode` | Tryb: `client` lub `server` | `client` |
| `-i` | `--mikrotik-ip` | IP Mikrotika (wymagane w trybie server) | (brak) |

## Automatyzacja / Automation

### Cron Job - Automatyczne Odnawianie Certyfikatów

#### Linux/macOS

```bash
# Edytuj crontab
crontab -e

# Dodaj wpisy dla automatycznego odnawiania
# Sprawdzaj codziennie o 2:00 AM
0 2 * * * /opt/pinpoint/bin/pinpoint -n pbabilas.client.vpn >> /var/log/pinpoint.log 2>&1

# Dla serwera
0 3 * * * /opt/pinpoint/bin/pinpoint -m server -n vpn.example.com -i 192.168.1.1 >> /var/log/pinpoint.log 2>&1
```

#### Systemd Timer (Rekomendowane)

```bash
# Utwórz plik usługi
sudo tee /etc/systemd/system/pinpoint.service << EOF
[Unit]
Description=PinPoint Certificate Renewal
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/pinpoint/bin/pinpoint -n pbabilas.client.vpn
StandardOutput=journal
StandardError=journal
EOF

# Utwórz timer
sudo tee /etc/systemd/system/pinpoint.timer << EOF
[Unit]
Description=PinPoint Certificate Renewal Timer
Requires=pinpoint.service

[Timer]
OnCalendar=daily
OnCalendar=02:00
Persistent=true

[Install]
WantedBy=timers.target
EOF

# Włącz timer
sudo systemctl enable pinpoint.timer
sudo systemctl start pinpoint.timer

# Sprawdź status
sudo systemctl status pinpoint.timer
sudo systemctl list-timers
```

## Struktura Katalogów / Directory Structure

```
ovpn-cert-renew/
├── main.go                      # Punkt wejścia
├── Makefile                     # Build configuration
├── go.mod                       # Go dependencies
├── go.sum
├── README.md                    # Ta dokumentacja
├── CLAUDE.md                    # Instrukcje dla Claude Code
├── internal/
│   ├── vault_client.go         # Integracja z Vault
│   ├── cert_db.go              # Baza danych certyfikatów
│   ├── server_manager.go       # Zarządzanie certyfikatami serwera
│   ├── mikrotik_integration.go # Integracja z Mikrotik
│   ├── mailer.go               # Wysyłanie emaili
│   ├── router_os.go            # Interfejs RouterOS
│   └── cert_manager.go         # Zarządzanie certyfikatami
├── conf/                        # Wygenerowane pliki .ovpn
├── certificates.json           # Baza danych (tworzona automatycznie)
├── user.ovpn.template          # Szablon konfiguracji OpenVPN
├── mail.template.html          # Szablon emaila
└── bin/                         # Skompilowane binarne
```

## Rozwiązywanie Problemów / Troubleshooting

### Problem: "Invalid AppRole credentials"

```
Rozwiązanie:
1. Sprawdź czy VAULT_ROLE_ID i VAULT_SECRET_ID są poprawne
2. Upewnij się że Secret ID nie wygasł (domyślnie 24 godziny)
   vault write -f auth/approle/role/ovpn-cert-renew/secret-id
3. Sprawdź połączenie do Vault:
   curl -k $VAULT_ADDR/v1/auth/approle/role/ovpn-cert-renew/role-id
```

### Problem: "Cannot connect to Mikrotik"

```
Rozwiązanie:
1. Sprawdź czy SSH jest włączony na routerze:
   /ip service print
2. Sprawdź dane logowania:
   ssh -v vpn-admin@192.168.1.1
3. Sprawdź czy host key jest zaakceptowany
4. Sprawdź logi na routerze:
   /log print where topics~"ssh"
```

### Problem: "SMTP connection failed"

```
Rozwiązanie:
1. Sprawdź zmienne SMTP w .env
2. Dla Gmail: użyj App Password, nie zwykłego hasła
3. Sprawdź czy port jest otwarty:
   telnet smtp.gmail.com 587
4. Sprawdź logi: sprawdź output narzędzia
```

### Problem: "Certificate already exists"

```
Rozwiązanie:
- To nie jest błąd! Narzędzie aktualizuje istniejące wpisy
- Jeśli chcesz wymuszić regenerację: --force-renew
```

## Praktyczne Przykłady / Practical Examples

### Scenariusz 1: Nowy Pracownik

```bash
# 1. Wygeneruj certyfikat
./bin/routeros-util.linux_amd64 \
  -n jan.kowalski.client.vpn \
  -e jan.kowalski@company.com

# 2. Poczekaj aż email zostanie wysłany
# 3. Pracownik pobiera plik .ovpn
# 4. Importuje go w OpenVPN clientem
```

### Scenariusz 2: Automaty Maintenance

```bash
#!/bin/bash
# Skrypt do automatycznego odnawiania wszystkich certyfikatów

USERS=(
  "pbabilas.client.vpn:pbabilas@example.com"
  "jan.kowalski.client.vpn:jan@example.com"
  "maria.nowak.client.vpn:maria@example.com"
)

for user_info in "${USERS[@]}"; do
  IFS=':' read -r name email <<< "$user_info"
  ./bin/pinpoint -n "$name" -e "$email"
done
```

### Scenariusz 3: Wielokrotne Serwery

```bash
#!/bin/bash
# Zarządzanie serwami na wielu routerach

SERVERS=(
  "vpn-pl.example.com:192.168.1.1"
  "vpn-de.example.com:192.168.2.1"
  "vpn-us.example.com:192.168.3.1"
)

for server_info in "${SERVERS[@]}"; do
  IFS=':' read -r name ip <<< "$server_info"
  ./bin/pinpoint \
    -m server \
    -n "$name" \
    -i "$ip" \
    -e admin@example.com
done
```

## Bezpieczeństwo / Security

⚠️ **Ważne uwagi bezpieczeństwa:**

1. **Secret ID**: Wygeneruj nowy Secret ID dla produkcji
   ```bash
   vault write -f auth/approle/role/ovpn-cert-renew/secret-id
   ```

2. **AppRole Token**: Ustaw krótki TTL
   ```bash
   vault write auth/approle/role/ovpn-cert-renew \
     token_ttl=1h \
     token_max_ttl=4h
   ```

3. **Uprawnienia**: Ograniczaj uprawnienia AppRole w polityce Vault

4. **SSH Keys**: Rozważ użycie kluczy SSH zamiast haseł dla Mikrotika
   ```bash
   /user ssh-keys import user=vpn-admin public-key-file=id_rsa.pub
   ```

5. **.env File**: Nigdy nie commituj pliku .env do repo
   - Dodaj do `.gitignore`
   - Przechowuj bezpiecznie (np. w systemie zarządzania sekreami)

6. **Logi**: Monitoruj logi dla podejrzanych aktywności
   ```bash
   tail -f /var/log/ovpn-renew.log
   ```

## Architektura / Architecture

```
┌─────────────────┐
│   CLI Usage     │
└────────┬────────┘
         │
    ┌────▼────────────────────┐
    │   Main Application      │
    │  (CLI Parsing & Routing)│
    └────┬────────────────────┘
         │
    ┌────▼──────────────────────────────┐
    │   VaultClient                     │
    │  (Issue & Renew Certificates)    │
    └────┬──────────────────────────────┘
         │
    ┌────▼──────────────────────────────┐
    │   CertificateDB                   │
    │  (JSON-based Persistent Storage)  │
    └────┬──────────────────────────────┘
         │
    ├────────────────────────┐
    │                        │
┌───▼────────────┐  ┌───────▼──────────┐
│   Mailer       │  │ MikrotikIntegration
│  (Email)       │  │  (SSH Upload)
└────────────────┘  └────────────────────┘
```

## Licencja / License

MIT License - patrz plik LICENSE

## Wsparcie / Support

Jeśli napotkasz problemy:

1. Sprawdź logi wyjścia
2. Zweryfikuj konfigurację Vault
3. Sprawdź połączenie sieciowe
4. Otwórz issue na GitHub z opisem problemu

## Kontrybuujący / Contributing

Chętnie przyjmujemy pull requests! Przed wysłaniem:

1. Przetestuj zmiany
2. Upewnij się że kod się kompiluje
3. Dodaj dokumentację jeśli potrzeba
4. Opisz zmiany w PR

---

**PinPoint** - Precise OpenVPN Certificate Management
