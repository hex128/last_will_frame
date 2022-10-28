FROM golang:alpine
WORKDIR $GOPATH/src/gitub.com/andrewshulgin/elevator_cam_bot
COPY . .
RUN apk add ffmpeg
RUN go get -d -v ./...
RUN go install -v ./...
CMD ["elevator_cam_bot"]
