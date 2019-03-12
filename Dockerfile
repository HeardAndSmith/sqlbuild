# Dockerfile, broken into stages.
# - base:    creates the base build/development environment
# - builder: runs tests and builds the binary
# - build:   the final sqlserver image with sqlbuild installed

# The code in this project expects that sql server is installed locally, so we
# use mssql-server-linux as our base image and then install a go development
# environment on top of that. The mssql-server-linux image uses ubuntu:16.04 as
# its base, so not too difficult.

ARG MSSQL_BASE_TAG=latest
ARG MSSQL_BUILD_TAG=latest

FROM mcr.microsoft.com/mssql/server:${MSSQL_BASE_TAG} AS base

# The lines following are reconstructed from the official golang:1.12-stretch
# Dockerfile. This needs to be manually changed whenever you want to update go.
# C'est la vie.
#
# Additionally the sql server image uses ubuntu (currently xenial, 16.04) as
# its base. For that reason we swapped out the buildpack-deps images from the
# go dockerfile from stretch to xenial.
# ----------------------------------------------------------------

# buildpack-deps:xenial-curl
# https://github.com/docker-library/buildpack-deps/blob/master/xenial/curl/Dockerfile

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    netbase \
    wget \
  && rm -rf /var/lib/apt/lists/*

RUN set -ex; \
  if ! command -v gpg > /dev/null; then \
    apt-get update; \
    apt-get install -y --no-install-recommends \
      gnupg \
      dirmngr \
    ; \
    rm -rf /var/lib/apt/lists/*; \
  fi

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

# golang:1.12-stretch
# https://github.com/docker-library/golang/blob/master/1.12/stretch/Dockerfile

# gcc for cgo
RUN apt-get update && apt-get install -y --no-install-recommends \
    g++ \
    gcc \
    libc6-dev \
    make \
    pkg-config \
  && rm -rf /var/lib/apt/lists/*

ENV GOLANG_VERSION 1.12

RUN set -eux; \
  \
# this "case" statement is generated via "update.sh"
  dpkgArch="$(dpkg --print-architecture)"; \
  case "${dpkgArch##*-}" in \
    amd64) goRelArch='linux-amd64'; goRelSha256='750a07fef8579ae4839458701f4df690e0b20b8bcce33b437e4df89c451b6f13' ;; \
    armhf) goRelArch='linux-armv6l'; goRelSha256='ea0636f055763d309437461b5817452419411eb1f598dc7f35999fae05bcb79a' ;; \
    arm64) goRelArch='linux-arm64'; goRelSha256='b7bf59c2f1ac48eb587817a2a30b02168ecc99635fc19b6e677cce01406e3fac' ;; \
    i386) goRelArch='linux-386'; goRelSha256='3ac1db65a6fa5c13f424b53ee181755429df0c33775733cede1e0d540440fd7b' ;; \
    ppc64el) goRelArch='linux-ppc64le'; goRelSha256='5be21e7035efa4a270802ea04fb104dc7a54e3492641ae44632170b93166fb68' ;; \
    s390x) goRelArch='linux-s390x'; goRelSha256='c0aef360b99ebb4b834db8b5b22777b73a11fa37b382121b24bf587c40603915' ;; \
    *) goRelArch='src'; goRelSha256='09c43d3336743866f2985f566db0520b36f4992aea2b4b2fd9f52f17049e88f2'; \
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

FROM mcr.microsoft.com/mssql/server:${MSSQL_BUILD_TAG} AS build
COPY --from=builder /go/bin/sqlbuild /usr/local/bin/sqlbuild
