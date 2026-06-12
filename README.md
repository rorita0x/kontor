# Kontor

Ein selbstgehostetes **Trading-Journal mit Risiko-Buchführung**. Der Name knüpft
an das historische *Kontor* an – das Handels- und Buchhaltungshaus eines
Kaufmanns – und steckt zugleich im *Konto*, dessen Stand die App über die
Trade-Ergebnisse mitführt.

Die Oberfläche ist auf Deutsch.

## Was es kann

- **Trades erfassen** – Symbol, Richtung (Long/Short/Skip), Ergebnis, Entry,
  Stop-Loss, Stückzahl bzw. Positionswert, Hebel/Margin, Notizen, Tags und
  Screenshots.
- **Positionsgröße aus dem Risiko ableiten** – Stückzahl, Positionswert, Risiko
  in € und Risiko in % vom Konto hängen zusammen; man gibt eines vor, der Rest
  wird berechnet.
- **Offenes Risiko überwachen** – Aggregation pro Asset und pro Asset-Klasse
  (Sektor) gegen pflegbare Limits, inkl. „noch frei"-Anzeige im Eintragsformular.
- **Kontostand über Exits verrechnen** – jeder Exit hält in der DB fest, ob er
  schon mit dem Kontostand verrechnet wurde. Beim Speichern wird nur die
  Differenz auf den Kontostand (Trading-Kapital) gebucht; offene Exits werden in
  Übersicht und Formular angemahnt. Der Kontostand ist die Basis für alle
  Risiko-Prozente.
- **Kennzahlen** – Win-Rate, offene Trades, offenes Risiko u. a.

## Stack

- **Backend:** Go mit [Gin](https://github.com/gin-gonic/gin)
- **Datenhaltung:** [BoltDB](https://github.com/etcd-io/bbolt) über
  [Storm](https://github.com/asdine/storm) – eine eingebettete Datei `trading.db`,
  kein externer Datenbankserver
- **Frontend:** serverseitige Go-Templates + Bootstrap, Alpine.js und HTMX
- **Uploads:** Screenshots landen unter `uploads/`

## Starten

Voraussetzung: eine aktuelle Go-Toolchain.

```bash
go run .
```

Der Server lauscht auf <http://127.0.0.1:18596>. Beim ersten Start werden die
Standard-Tags, Asset-Klassen und ein Settings-Datensatz angelegt.

Alternativ startet `./launch.sh` den Server und öffnet ihn in einem eigenen
Firefox-Profil; beim Schließen des Fensters wird der Server wieder beendet. Für
den Desktop liegt zusätzlich `trading-db.desktop` bei.

> Das Firefox Profil muss manuell angelegt werden, wenn man die Verknüpfung nutzen will!

## Tests

```bash
go test ./...
```

Abgedeckt sind die Trade-/Risiko-Logik und die Verrechnungs-Buchung über
`/insert` (Letzteres als Handler-Test gegen eine isolierte Temp-DB).

## Daten

`trading.db` ist das echte Journal und wird – wie `uploads/` – von Git
ignoriert. Für Tests immer eine eigene Temp-DB verwenden, nie gegen `trading.db`
schreiben.
