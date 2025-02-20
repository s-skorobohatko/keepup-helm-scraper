FROM golang:1.23 AS builder
WORKDIR /app

COPY main.go .
RUN go mod init helm-scraper && go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o helm-scraper main.go

FROM alpine:latest
WORKDIR /root/

#ENV CLUSTER_NAME="minikube"
COPY --from=builder /app/helm-scraper /root/helm-scraper
RUN chmod +x /root/helm-scraper

ENTRYPOINT ["/root/helm-scraper"]
