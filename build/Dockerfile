FROM docker.io/library/golang:1.13-alpine3.10 AS build
# Populate build cache
COPY go.mod go.sum /go/src/github.com/mgoltzsche/credential-manager/
WORKDIR /go/src/github.com/mgoltzsche/credential-manager
#RUN printf 'package main\nfunc main() {}' > main.go
#RUN go build .
RUN go mod download
# Do actual build
COPY . /go/src/github.com/mgoltzsche/credential-manager
RUN go build -ldflags '-s -w -extldflags "-static"' ./cmd/manager && mv manager /bin/manager

FROM alpine:3.10
ENV OPERATOR=/usr/local/bin/credential-manager \
    USER_UID=1001 \
    USER_NAME=credential-manager
COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup
USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/entrypoint"]
COPY --from=build /bin/manager ${OPERATOR}
