# Dockerfile, broken into stages.
# - base:    creates the base build/development environment
# - builder: runs tests and builds the binary
# - build:   the final sqlserver image with sqlbuild installed

# The code in this project expects that sql server is installed locally, so we
# use mssql-server-linux as our base image and then install a go development
# environment on top of that. The mssql-server-linux image uses ubuntu:16.04 as
# its base, so not too difficult.

ARG MSSQL_BASE_TAG=2017-CU10
ARG MSSQL_BUILD_TAG=latest

FROM microsoft/mssql-server-linux:${MSSQL_BASE_TAG} AS base

# The lines following are reconstructed from the official golang:1.11-stretch
# Dockerfile. This needs to be manually changed whenever you want to update go.
# C'est la vie.
# ----------------------------------------------------------------

# buildpack-deps:xenial-curl
# https://github.com/docker-library/buildpack-deps/blob/master/xenial/curl/Dockerfile

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    wget \
&& rm -rf /var/lib/apt/lists/*

# buildpack-deps:xenial-scm
# https://github.com/docker-library/buildpack-deps/blob/master/xenial/scm/Dockerfile

# procps is very common in build systems, and is a reasonably small package
RUN apt-get update && apt-get install -y --no-install-recommends \
    bzr \
    git \
    mercurial \
    openssh-client \
    subversion \
    \
    procps \
&& rm -rf /var/lib/apt/lists/*

# golang:1.11-stretch
# https://github.com/docker-library/golang/blob/master/1.11/stretch/Dockerfile

# gcc for cgo
RUN apt-get update && apt-get install -y --no-install-recommends \
    g++ \
    gcc \
    libc6-dev \
    make \
    pkg-config \
&& rm -rf /var/lib/apt/lists/*

ENV GOLANG_VERSION 1.11

RUN set -eux; \
  \
# this "case" statement is generated via "update.sh"
  dpkgArch="$(dpkg --print-architecture)"; \
  case "${dpkgArch##*-}" in \
    amd64) goRelArch='linux-amd64'; goRelSha256='b3fcf280ff86558e0559e185b601c9eade0fd24c900b4c63cd14d1d38613e499' ;; \
    armhf) goRelArch='linux-armv6l'; goRelSha256='8ffeb3577d8ca5477064f1cb8739835973c866487f2bf81df1227eaa96826acd' ;; \
    arm64) goRelArch='linux-arm64'; goRelSha256='e4853168f41d0bea65e4d38f992a2d44b58552605f623640c5ead89d515c56c9' ;; \
    i386) goRelArch='linux-386'; goRelSha256='1a91932b65b4af2f84ef2dce10d790e6a0d3d22c9ea1bdf3d8c4d9279dfa680e' ;; \
    ppc64el) goRelArch='linux-ppc64le'; goRelSha256='e874d617f0e322f8c2dda8c23ea3a2ea21d5dfe7177abb1f8b6a0ac7cd653272' ;; \
    s390x) goRelArch='linux-s390x'; goRelSha256='c113495fbb175d6beb1b881750de1dd034c7ae8657c30b3de8808032c9af0a15' ;; \
    *) goRelArch='src'; goRelSha256='afc1e12f5fe49a471e3aae7d906c73e9d5b1fdd36d52d72652dde8f6250152fb'; \
      echo >&2; echo >&2 "warning: current architecture ($dpkgArch) does not have a corresponding Go binary release; will be building from source"; echo >&2 ;; \
  esac; \
  \
  url="https://golang.org/dl/go${GOLANG_VERSION}.${goRelArch}.tar.gz"; \
  wget -O go.tgz "$url"; \
  echo "${goRelSha256} *go.tgz" | sha256sum -c -; \
  tar -C /usr/local -xzf go.tgz; \
  rm go.tgz; \
  \
  if [ "$goRelArch" = 'src' ]; then \
    echo >&2; \
    echo >&2 'error: UNIMPLEMENTED'; \
    echo >&2 'TODO install golang-any from jessie-backports for GOROOT_BOOTSTRAP (and uninstall after build)'; \
    echo >&2; \
    exit 1; \
  fi; \
  \
  export PATH="/usr/local/go/bin:$PATH"; \
  go version

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"
WORKDIR $GOPATH

# ----------------------------------------------------------------
# End of the go dockerfile code.

FROM base AS builder
WORKDIR /code/
COPY go.mod go.sum /code/
RUN go mod download
COPY . .
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go install -a -installsuffix cgo .

FROM microsoft/mssql-server-linux:${MSSQL_BUILD_TAG} AS build
COPY --from=builder /go/bin/sqlbuild /usr/local/bin/sqlbuild
