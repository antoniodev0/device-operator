# gateway/Dockerfile

# --- Fase 1: Build ---
FROM golang:1.24-alpine AS builder

WORKDIR /app

# *** MODIFICA CHIAVE QUI ***
# Copiamo i file go.mod e go.sum dalla sottocartella 'gateway'.
COPY gateway/go.mod gateway/go.sum ./
RUN go mod download

# Copiamo tutto il codice sorgente dalla sottocartella 'gateway'.
COPY gateway/ ./

# Compiliamo il codice. Ora i file .go sono presenti in /app.
RUN CGO_ENABLED=0 GOOS=linux go build -o /gateway .


# --- Fase 2: Immagine Finale ---
FROM alpine:3.18

RUN apk add --no-cache ca-certificates

COPY --from=builder /gateway /gateway

ENTRYPOINT ["/gateway"]