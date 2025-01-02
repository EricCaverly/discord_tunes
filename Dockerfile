FROM golang:1.23.4

WORKDIR /app

COPY ./app .

RUN apt-get update -y && apt-get install -y ffmpeg bash

RUN go mod download

RUN go build -o ./discord_app

CMD ["./discord_app"]
