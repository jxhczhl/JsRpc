FROM golang:1.16 as builder
# Setting environment variables
ENV GOPROXY="https://goproxy.cn,direct" \
    GO111MODULE="on" \
    CGO_ENABLED="0" \
    GOOS="linux" \
    GOARCH="amd64"

# Switch to workspace
WORKDIR /go/src/github.com/gowebspider/jsrpc/
# Load file
COPY . .
# add rely
# Build and place the results in /tmp/jsrpc
RUN  go mod tidy && go build -o /tmp/jsrpc .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /tmp/jsrpc .
EXPOSE 12080 12443
CMD ["./jsrpc"]