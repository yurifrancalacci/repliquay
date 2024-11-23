FROM docker.io/library/golang:1.23 AS builder
COPY . .
RUN go build -o /build/repliquay main.go

FROM redhat/ubi9-micro
COPY --from=builder /build/repliquay /usr/local/bin/repliquay
USER 1001
CMD [ "/usr/local/bin/repliquay" ]