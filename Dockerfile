FROM alpine:3.13

COPY bot bot
COPY data/ data/

CMD "./bot"
