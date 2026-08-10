# ---- build stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /out/guardrail ./cmd/guardrail

# ---- runtime stage ----
FROM alpine:latest

WORKDIR /app

COPY --from=builder /out/guardrail /app/guardrail
COPY policy.yaml /app/policy.yaml

EXPOSE 8080

CMD ["/app/guardrail", "serve", "-policy", "/app/policy.yaml"]
