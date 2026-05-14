FROM golang:1.26 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /richmond .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /richmond /richmond

USER nonroot:nonroot

CMD ["richmond", "sync"]
