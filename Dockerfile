# This is a multi-stage Dockerfile. This requires Docker 17.05 or later
#
# Step #1 Run unit tests and build an executable that doesn't require the go libs
FROM golang as builder
WORKDIR /work
ADD . .
RUN go get -d -v
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /observerip-proxy .
#
# Step #2: Copy the executable into a minimal image (less than 5MB) 
#         which doesn't contain the build tools and artifacts
FROM alpine:latest  
RUN apk --no-cache add ca-certificates
COPY --from=builder /observerip-proxy /observerip-proxy
CMD ["/observerip-proxy"] 