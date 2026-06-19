# Build from repo root: docker build -f apps/gateway/Dockerfile .
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY apps/gateway/go.mod apps/gateway/go.sum* ./apps/gateway/
WORKDIR /src/apps/gateway
RUN go mod download
COPY apps/gateway/ ./
COPY connectors/ /src/connectors/
ENV COUPLER_CONNECTORS_DIR=/src/connectors
RUN CGO_ENABLED=0 go build -o /coupler-gateway ./cmd/gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /coupler-gateway /coupler-gateway
COPY connectors/ /connectors/
ENV COUPLER_CONNECTORS_DIR=/connectors
ENV PORT=8787
EXPOSE 8787
USER nonroot:nonroot
ENTRYPOINT ["/coupler-gateway"]
