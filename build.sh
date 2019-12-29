#!/bin/sh

set -e

docker build -t secret-operator-build -f - . <<-EOF
FROM golang:1.13-alpine3.10
RUN apk add --update --no-cache curl make git gcc libc-dev
RUN curl -fsSL https://github.com/tianon/gosu/releases/download/1.11/gosu-amd64 > /usr/local/bin/gosu && chmod +x /usr/local/bin/gosu
ENV OPERATOR_SDK_VERSION=v0.13.0
#RUN curl -fsSL https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu > /usr/local/bin/operator-sdk && chmod +x /usr/local/bin/operator-sdk
RUN git clone https://github.com/operator-framework/operator-sdk.git /go/src/github.com/operator-framework/operator-sdk
WORKDIR /go/src/github.com/operator-framework/operator-sdk
RUN git checkout $OPERATOR_SDK_VERSION
RUN go build -o /usr/local/bin/operator-sdk github.com/operator-framework/operator-sdk/cmd/operator-sdk
WORKDIR /go
#RUn go get -d github.com/operator-framework/operator-sdk
EOF

DIR=/go/src/github.com/mgoltzsche/credential-manager
#docker run --rm -ti -v "`pwd`:$DIR" -w $DIR -e USR="$(id -u)" --entrypoint /bin/sh secret-operator-build -c "chown -R ${USR}:${USR} /go && /bin/bash # gosu $USR:$USR make"

docker run --rm -ti -v "`pwd`:$DIR" -w $DIR -e USR="$(id -u)" --entrypoint /bin/sh secret-operator-build -c "sh"
