FROM golang:1.21-alpine AS builder

WORKDIR /app

# Certificats CA nécessaires pour les appels HTTPS
RUN apk add --no-cache ca-certificates

# Copier tous les fichiers sources
COPY . .

# Télécharger les dépendances et construire le binaire
# go mod tidy regénère go.sum si nécessaire
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o weather-discord .

# ---

FROM alpine:3.19

WORKDIR /app

# ca-certificates pour HTTPS + tzdata pour les fuseaux horaires dans les logs
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/weather-discord .

ENTRYPOINT ["/app/weather-discord"]
