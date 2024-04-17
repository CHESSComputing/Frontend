FROM golang:latest as go-builder
MAINTAINER Valentin Kuznetsov vkuznet@gmail.com

# build procedure
ENV PROJECT=Frontend
ENV WDIR=/data
WORKDIR $WDIR
RUN mkdir /build
RUN git clone https://github.com/CHESSComputing/$PROJECT
ARG CGO_ENABLED=0
RUN cd $PROJECT && make && cp srv /build && cp -r static /build

# build final image for given image
FROM alpine as final
# FROM gcr.io/distroless/static as final
RUN mkdir -p /data
COPY --from=go-builder /build/srv /data
COPY --from=go-builder /build/static /data/static
LABEL org.opencontainers.image.description="FOXDEN Frontend service"
LABEL org.opencontainers.image.source=https://github.com/chesscomputing/frontend
LABEL org.opencontainers.image.licenses=MIT
WORKDIR /data
