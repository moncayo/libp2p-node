FROM golang:1.23rc1-alpine3.20 as build

WORKDIR /app

RUN apk --no-cache add ca-certificates

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /libp2p-node

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /libp2p-node /libp2p-node

ENTRYPOINT [ "/libp2p-node" ]
