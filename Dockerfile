FROM golang:1.21 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o gpu-exporter .

FROM ubuntu:22.04
COPY --from=build /app/gpu-exporter /gpu-exporter
CMD ["/gpu-exporter"]
ENV NVIDIA_VISIBLE_DEVICES=all
EXPOSE 9445
