FROM gliderlabs/logspout:master

ENV GOPATH=/go
RUN apk add --update go build-base git mercurial ca-certificates
RUN mkdir -p /go/src/github.com/gliderlabs
RUN go get github.com/Masterminds/glide && $GOPATH/bin/glide install
# docker build -t darkdarkdragon/logspout-loki -f Dockerfile .