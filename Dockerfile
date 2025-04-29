FROM golang:alpine
WORKDIR $GOPATH/src/gitub.com/hex128/last_will_frame
RUN apk add --no-cache ffmpeg
COPY go.mod .
COPY main.go .
RUN go get -d -v ./...
RUN go install -v ./...
CMD ["last_will_frame"]
