FROM golang:1.21.1

EXPOSE 8001 8002 8003 8005

RUN mkdir -p /app
WORKDIR /app

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .
RUN go install ./...

CMD ["kairos"]
