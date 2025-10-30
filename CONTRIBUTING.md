# Contributing to OpenVPN Certificate Manager

Dziękujemy za zainteresowanie wkładem w ten projekt! / Thank you for your interest in contributing to this project!

## Jak Zaangażować Się / How to Get Involved

### Reportowanie Błędów / Reporting Bugs

Znalazłeś błąd? Otwórz issue z:
- Jasnym opisem problemu
- Krokami do reprodukcji
- Oczekiwanym vs aktualnym zachowaniem
- Informacjami o środowisku (system, wersja Go, etc.)

### Sugestie Funkcjonalności / Feature Requests

Masz pomysł na nową funkcję? Otwórz issue z:
- Opisem use case
- Propozycją implementacji (opcjonalnie)
- Możliwymi alternatywami

### Pull Requests

1. **Fork** tego repozytorium
2. **Clone** go lokalnie
   ```bash
   git clone https://github.com/your-username/ovpn-cert-renew.git
   cd ovpn-cert-renew
   ```

3. **Utwórz nową gałąź** dla twojej funkcji
   ```bash
   git checkout -b feature/your-feature-name
   ```

4. **Zainstaluj zależności**
   ```bash
   go mod download
   ```

5. **Wprowadź zmiany** w swoim kodzie

6. **Testuj** swoje zmiany
   ```bash
   go test ./...
   go build
   ```

7. **Commit** z jasnym opisem
   ```bash
   git commit -m "Add feature: clear description of changes"
   ```

8. **Push** do swojego fork
   ```bash
   git push origin feature/your-feature-name
   ```

9. **Otwórz Pull Request** z opisem zmian

## Wytyczne Kodowania / Code Guidelines

### Styl Kodu

- Przestrzegaj standardów Go (`gofmt`, `golint`)
- Pisz czytelny i dobrze dokumentowany kod
- Dodaj komentarze do skomplikowanej logiki
- Używaj sensownych nazw zmiennych i funkcji

### Commits

- Jeden commit = jeden logiczny zmiana
- Pisz opisowe wiadomości commit
- Format: `type: brief description`

Typy:
- `feat:` - nowa funkcja
- `fix:` - naprawa błędu
- `docs:` - dokumentacja
- `refactor:` - refaktoryzacja kodu
- `test:` - dodanie/zmiana testów
- `ci:` - zmiana konfiguracji CI

Przykład:
```
feat: add support for custom certificate paths

Add --cert-path flag to allow users to specify custom
certificate storage location.
```

### Testowanie

- Dodaj testy dla nowych funkcji
- Upewnij się że wszystkie testy przechodzą
- Testuj zarówno happy path jak i edge cases

```bash
go test -v ./...
go test -cover ./...
```

## Struktura Projektu / Project Structure

```
internal/
├── vault_client.go         # Vault API interactions
├── cert_db.go              # Certificate storage
├── server_manager.go       # Server certificate operations
├── mikrotik_integration.go # Mikrotik communication
├── mailer.go               # Email handling
├── router_os.go            # Router OS interface
└── cert_manager.go         # Certificate management

main.go                      # Application entry point
go.mod / go.sum             # Dependency management
Makefile                    # Build configuration
```

## Dla Maintainerów / For Maintainers

### Merging PRs

1. Zweryfikuj że PR przechodzi wszystkie testy
2. Sprawdź czy kod jest dobrze dokumentowany
3. Sprawdź czy zmiany są zgodne z architekturą
4. Dodaj labele (bug, enhancement, documentation, etc.)
5. Merge PR

### Releases

```bash
# Tag release
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0

# GitHub Actions automatycznie opublikuje release
```

## Bezpieczeństwo / Security

⚠️ **WAŻNE**: Jeśli znajdujesz lukę w bezpieczeństwie:
1. **NIE** otwieraj public issue
2. Skontaktuj się z maintainerem bezpośrednio
3. Pozwól nam opublikować poprawkę przed ujawnieniem

## Licencja / License

Poprzez wkład w ten projekt, zgadzasz się że twoja praca będzie licencjonowana na warunkach MIT License.

## Pytania?

- 📧 Skontaktuj się z maintainerem
- 💬 Otwórz dyskusję na GitHub
- 📖 Sprawdź istniejące issues

Dziękujemy za wkład! / Thank you for contributing!
