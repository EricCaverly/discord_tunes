FROM golang:1.23.4-alpine3.21

WORKDIR /app

COPY ./app .

RUN go mod download

RUN go build -o ./discord_app

CMD ["./discord_app"]
