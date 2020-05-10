FROM golang:alpine AS builder

ENV GO111MODULE=on
ENV GOPATH=""

WORKDIR /app
#COPY *.go .
#COPY go.mod .
#COPY go.sum .
COPY . .

RUN go build -o controller .

FROM alpine

COPY --from=builder /app/controller /app/

CMD ["./app/controller"]
