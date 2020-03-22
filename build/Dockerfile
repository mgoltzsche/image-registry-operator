FROM docker.io/library/golang:1.14-alpine3.11 AS builddeps
RUN apk add --update --no-cache curl make gcc libc-dev
COPY go.mod go.sum /go/src/github.com/mgoltzsche/credential-manager/
WORKDIR /go/src/github.com/mgoltzsche/credential-manager
RUN go mod download

FROM builddeps AS build
COPY version /go/src/github.com/mgoltzsche/credential-manager/version
COPY pkg /go/src/github.com/mgoltzsche/credential-manager/pkg
COPY cmd /go/src/github.com/mgoltzsche/credential-manager/cmd
RUN go build -ldflags '-s -w -extldflags "-static"' ./cmd/manager && mv manager /bin/

# Prepare operator image
FROM alpine:3.11 AS operator
COPY build/bin /usr/local/bin
ENV OPERATOR=/usr/local/bin/credential-manager \
    USER_UID=1001 \
    USER_NAME=credential-manager
RUN /usr/local/bin/user_setup
USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/entrypoint"]
COPY --from=build /bin/manager ${OPERATOR}

FROM build AS test
RUN go test -v ./pkg/...

FROM operator