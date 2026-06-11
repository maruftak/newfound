FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/newfound ./cmd/newfound

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/newfound /usr/local/bin/newfound
ENTRYPOINT ["newfound"]
