FROM alpine:3.15.11

COPY db/migrations db/migrations
COPY bot bot

CMD ["./bot"]
