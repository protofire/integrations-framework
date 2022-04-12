# Necessary as the Celo library uses CGO
FROM golang:1.17 as builder

# Setup app folder
RUN mkdir /app
ADD . /app
WORKDIR /app

# Build the app
RUN go mod download && \
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go test -c ./suite/soak/tests -o ./remote.test && \
    chmod +x ./remote.test

# Move to smaller image
FROM golang:1.17
COPY --from=builder /app/remote.test /root/remote.test
