FROM docker.io/library/golang:1.14-alpine3.11 AS builddeps
RUN apk add --update --no-cache curl make gcc libc-dev
COPY go.mod go.sum tools.go /go/src/github.com/mgoltzsche/image-registry-operator/
WORKDIR /go/src/github.com/mgoltzsche/image-registry-operator
RUN go mod download
RUN cat tools.go | grep -E '^\s*_' | cut -d'"' -f2 | xargs -n1 go install

FROM builddeps AS build
COPY version /go/src/github.com/mgoltzsche/image-registry-operator/version
COPY pkg /go/src/github.com/mgoltzsche/image-registry-operator/pkg
COPY cmd /go/src/github.com/mgoltzsche/image-registry-operator/cmd
RUN go build -ldflags '-s -w -extldflags "-static"' ./cmd/manager && mv manager /bin/

# Prepare operator image
FROM alpine:3.11 AS operator
COPY build/bin /usr/local/bin
ENV OPERATOR=/usr/local/bin/image-registry-operator \
    USER_UID=1001 \
    USER_NAME=image-registry-operator
RUN /usr/local/bin/user_setup
USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/entrypoint"]
COPY --from=build /bin/manager ${OPERATOR}

FROM build AS test
RUN go test -v ./pkg/...

FROM operator