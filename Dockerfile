# Build stage
FROM golang:1.21.4-alpine3.17 AS build-stage
ENV GO111MODULE=auto
ENV CGO_ENABLED=1
ENV GOOS=linux
RUN apk add --no-cache --update go gcc g++
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o main main.go

VOLUME [ "/data" ]

# Run stage
FROM alpine:3.17
WORKDIR /app
COPY --from=build-stage /app/templates ./templates/
COPY --from=build-stage /app/data ./data/
COPY --from=build-stage /app/main .

EXPOSE 80

CMD ["/app/main"]