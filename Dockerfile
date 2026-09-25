FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /vigil ./cmd/vigil

FROM gcr.io/distroless/static:nonroot
COPY --from=build /vigil /vigil
EXPOSE 8080
ENTRYPOINT ["/vigil", "-config", "/etc/vigil/vigil.yaml"]
