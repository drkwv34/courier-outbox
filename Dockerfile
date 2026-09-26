# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o /out/courier ./cmd/courier \
 && go build -ldflags="-s -w" -o /out/mock-subscriber ./cmd/mock-subscriber

FROM gcr.io/distroless/static-debian12:nonroot AS mock-subscriber
COPY --from=build /out/mock-subscriber /usr/local/bin/mock-subscriber
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/mock-subscriber"]

FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build /out/courier /usr/local/bin/courier
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/courier"]
CMD ["serve"]
