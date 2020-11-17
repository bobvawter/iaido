FROM golang:1.15 AS builder
WORKDIR /tmp/compile
COPY . .
RUN make lint testrace build

FROM scratch
ENTRYPOINT ["/usr/bin/iaido", "--config", "/data/iaido.yaml"]
COPY --from=builder /tmp/compile/bin/iaido /usr/bin/iaido
