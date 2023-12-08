FROM golang:1.21.1

EXPOSE 8003

RUN mkdir -p /app
WORKDIR /app

COPY ../../../../go.mod . 
COPY ../../../../go.sum .
RUN go mod download

COPY . .
RUN go install ./...

CMD ["server"]
