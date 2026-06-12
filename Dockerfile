# syntax=docker/dockerfile:1

# ---- Build stage ----------------------------------------------------------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Abhängigkeiten zuerst – wird nur neu gebaut, wenn sich go.mod/go.sum ändert.
COPY go.mod go.sum ./
RUN go mod download

# Restlichen Quellcode kopieren und statisch bauen (kein CGO -> läuft auf scratch).
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /trading-db .

# ---- Runtime stage --------------------------------------------------------
FROM gcr.io/distroless/static-debian12

WORKDIR /app

# Binary + zur Laufzeit benötigte (relative) Ressourcen.
COPY --from=build /trading-db /app/trading-db
COPY --from=build /src/templates ./templates
COPY --from=build /src/assets ./assets

# Daten liegen in Volumes (siehe docker-compose.yml).
ENV LISTEN_ADDR=0.0.0.0:18596
EXPOSE 18596

ENTRYPOINT ["/app/trading-db"]
