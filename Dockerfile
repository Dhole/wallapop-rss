FROM golang:alpine as builder
RUN mkdir /build 
ADD . /build/
WORKDIR /build 
RUN addgroup -S build
RUN adduser -S -D -H -h /build -G build build
RUN chown build:build -R /build
USER build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o main .

FROM alpine
RUN whoami
COPY --from=builder /build/main /app/wallapop-rss
WORKDIR /app
CMD ["./wallapop-rss"]
