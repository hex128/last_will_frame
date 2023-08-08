FROM golang:alpine
WORKDIR $GOPATH/src/gitub.com/andrewshulgin/elevator_cam_bot
RUN apk add --no-cache ffmpeg
COPY go.mod .
COPY main.go .
RUN go get -d -v ./...
RUN go install -v ./...
CMD ["elevator_cam_bot"]
