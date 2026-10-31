# Build from the cms/ folder (the example uses the SDK in sdk/go):
#   docker build -f examples/service-plugins/hello-service/Dockerfile -t hello-service .
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY sdk/go/ sdk/go/
COPY examples/service-plugins/hello-service/service/ examples/service-plugins/hello-service/service/
WORKDIR /src/examples/service-plugins/hello-service/service
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hello-service .

FROM alpine:3.22
RUN adduser -D app
COPY --from=build /out/hello-service /app/hello-service
USER app
ENV ADDR=:9300
EXPOSE 9300
ENTRYPOINT ["/app/hello-service"]
