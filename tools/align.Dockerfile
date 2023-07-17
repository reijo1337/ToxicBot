FROM golang:1.20-alpine

RUN apk add --no-cache build-base
RUN go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest

WORKDIR /app