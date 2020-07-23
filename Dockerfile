FROM golang:alpine as builder

WORKDIR /app
RUN apk update --no-cache && apk add ca-certificates
COPY k8s-event /app/k8s-event

CMD ["/app/k8s-event", "start"]
