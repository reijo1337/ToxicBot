FROM alpine:3.13

COPY go-binary bot
CMD "./bot"