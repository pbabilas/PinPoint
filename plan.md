   ## Główne zmiany:

       1. Nowa struktura danych dla certyfikatów serwera - Rozszerzyłem bazę danych o nowe pola:


           * ServerCertificate - struktura przechowująca informacje o certyfikatach serwera

           * Servers - mapa certyfikatów serwera w bazie danych

           * Metody CRUD dla certyfikatów serwera
       2. Nowy moduł ServerManager - Stworzyłem dedykowany moduł do zarządzania certyfikatami serwera:


           * SetupServerCertificate - konfiguracja nowego certyfikatu serwera

           * RenewServerCertificate - odnawianie certyfikatu serwera

           * CheckServerCertificateExpiry - sprawdzanie ważności certyfikatu serwera

           * GetAllServers - pobieranie wszystkich certyfikatów serwera

           * DeleteServerCertificate - usuwanie certyfikatu serwera
       3. Integracja z Vault dla certyfikatów serwera - Rozszerzyłem klienta Vault o nowe funkcje:


           * IssueServerCertificate - generowanie certyfikatu serwera z odpowiednimi uprawnieniami

           * GetServerCertificate - pobieranie informacji o certyfikacie serwera z Vault
       4. Nowy tryb pracy --mode=server - Dodałem flagę trybu pracy:


           * --mode=client (domyślny) - zarządzanie certyfikatami klienta

           * --mode=server - zarządzanie certyfikatami serwera

           * --mikrotik-ip - adres IP routera Mikrotik (wymagany w trybie serwera)
       5. Inteligentne zarządzanie emailami - Aplikacja teraz wysyła emaile tylko w określonych przypadkach:


           * Gdy certyfikat został odnowiony

           * Gdy użyto flagi --resend

           * Nie wysyła emaila przy każdym uruchomieniu
       6. Mechanizm aktualizacji certyfikatu serwera - Dodałem logikę do:


           * Wykrywania, gdy certyfikat serwera wymaga aktualizacji

           * Informowania o potrzebie aktualizacji na routerze Mikrotik

           * Przygotowania do przyszłej integracji z API Mikrotika

   ## Architektura rozwiązania:

   Zaimplementowałem architekturę opartą na integracji z systemem konfiguracji Vault, która zapewnia:

       * Pełną automatyzację - minimalna interwencja ręczna
       * Skalowalność - wykorzystanie natywnych mechanizmów Vault
       * Bezpieczeństwo - certyfikaty nigdy nie opuszczają Vault
       * Integrację - łatwa integracja z istniejącą infrastrukturą

   ## Jak używać:

   ### Tryb klienta (domyślny):

       # Standardowe użycie
       ./bin/ovpn-cert-renew -n pbabilas.client.vpn

       # Wymuszone odnowienie
       ./bin/ovpn-cert-renew -n pbabilas.client.vpn --force-renew

       # Ponowne wysłanie emaila
       ./bin/ovpn-cert-renew -n pbabilas.client.vpn --resend

   ### Tryb serwera:

       # Generowanie nowego certyfikatu serwera
       ./bin/ovpn-cert-renew -m server -n vpn.example.com -i 192.168.1.1 -e admin@example.com

       # Wymuszone odnowienie certyfikatu serwera
       ./bin/ovpn-cert-renew -m server -n vpn.example.com -i 192.168.1.1 -e admin@example.com --force-renew

   ## Rozwiązane problemy:

   ✅ Integracja z Vault - pełna integracja z systemem konfiguracji Vault
   ✅ Zarządzanie certyfikatami serwera - dedykowany moduł do zarządzania certyfikatami serwera
   ✅ Inteligentne wysyłanie emaili - tylko gdy jest to naprawdę potrzebne
   ✅ Tryb pracy - możliwość wyboru między trybem klienta i serwera
   ✅ Mechanizm aktualizacji - przygotowanie do integracji z Mikrotikiem

   ## Kolejne kroki rozwoju:

       1. Implementacja Vault Agent - automatyczna aktualizacja certyfikatów na serwerze
       2. Integracja z API Mikrotika - automatyczna aktualizacja certyfikatu serwera
       3. Rozszerzone monitorowanie - ciągłe monitorowanie ważności certyfikatów
       4. Dashboard zarządzania - interfejs webowy do zarządzania certyfikatami

   Narzędzie jest teraz gotowe do zarządzania zarówno certyfikatami klienta, jak i serwera OpenVPN z pełną integracją z Vault i mechanizmami automatyzacji.
