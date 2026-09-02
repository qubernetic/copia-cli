FROM golang:1.27.1-alpine

RUN apk add --no-cache git build-base

RUN git config --global --add safe.directory /app

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
