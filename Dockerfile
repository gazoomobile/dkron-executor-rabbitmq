# build dkron-executor-rabbitmq
FROM golang:1.11-alpine AS builder

ARG dkron_version=1.1.1

# install packages and download binaries
RUN apk --no-cache add git \
  && wget -O - https://raw.githubusercontent.com/golang/dep/master/install.sh | sh \
  && wget -O - https://github.com/victorcoder/dkron/releases/download/v${dkron_version}/dkron_${dkron_version}_linux_amd64.tar.gz | tar xzf - \
  && mv dkron /tmp/dkron

# ensure dependencies
WORKDIR $GOPATH/src/github.com/gazoomobile/dkron-executor-rabbitmq
COPY Gopkg.toml Gopkg.lock ./

RUN dep ensure -vendor-only -v

# compile and install
COPY . ./
RUN go install -v github.com/gazoomobile/dkron-executor-rabbitmq

# final image
# re-use the pre-build dkron image but add in the built executor
FROM dkron/dkron:v1.1.1
ENV SHELL /bin/bash

COPY --from=builder /go/bin/dkron-executor-rabbitmq /opt/local/dkron/dkron-executor-rabbitmq

