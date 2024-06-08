##
##  Build
##
FROM golang:1.22-bookworm AS build

# Install dependencies
ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && \
    apt-get install --assume-yes --no-install-recommends \
		build-essential \
		libgif-dev \
		libgs-dev \
		libjpeg-dev \
		libpng-dev \
		libtiff-dev \
		libx11-dev \
		pkg-config \
		wget && \
	rm -rf /var/lib/apt/lists/*

# Build ImageMagick
ENV IMAGEMAGICK_VERSION 7.1.1-33

RUN cd && \
	wget https://github.com/ImageMagick/ImageMagick/archive/${IMAGEMAGICK_VERSION}.tar.gz && \
	tar xvzf ${IMAGEMAGICK_VERSION}.tar.gz && \
	cd ImageMagick* && \
	./configure \
	    --without-magick-plus-plus \
	    --without-perl \
	    --disable-openmp \
	    --with-gvc=no \
	    --disable-docs && \
	make -j$(nproc) && \
	make install && \
	ldconfig /usr/local/lib

# Build Go app
WORKDIR /app
COPY . .

RUN go build -ldflags="-s -w -X 'main.Version=$(git describe --tag)'" -o magick-server main.go

##
##  Deploy
##
FROM debian:bookworm AS deploy

LABEL org.opencontainers.image.authors="Dr. Thomas Jansen <thomas@crissyfield.de>"
LABEL org.opencontainers.image.vendor="Crissy Field GmbH"

# Install dependencies
ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && \
    apt-get install --assume-yes \
		ghostscript \
		imagemagick \
		tini \
		tzdata && \
	rm -rf /var/lib/apt/lists/*

# Copy and setup shared libraries
COPY --from=build /usr/local/lib /usr/local/lib
RUN ldconfig /usr/local/lib

# Copy and run Go app
COPY --from=build /app/magick-server /magick-server

ENTRYPOINT [ "/usr/bin/tini", "--", "/magick-server" ]
