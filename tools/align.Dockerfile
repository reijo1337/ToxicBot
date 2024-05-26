FROM golang:1.22-alpine

RUN apk add --no-cache build-base
RUN go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest

WORKDIR /app