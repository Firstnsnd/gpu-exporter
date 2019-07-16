FROM golang as build
RUN  go get github.com/vaniot-s/gpu-exporter

FROM ubuntu:18.04
COPY --from=build /go/bin/gpu-exporter /
CMD /gpu-exporter
ENV NVIDIA_VISIBLE_DEVICES=all
EXPOSE 9445
