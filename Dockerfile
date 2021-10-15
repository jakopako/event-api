FROM golang:1.16-alpine AS builder

# Move to working directory (/build).
WORKDIR /build

# Copy and download dependency using go mod.
COPY go.mod go.sum ./
RUN go mod download

# Copy the code into the container.
COPY . .

# Set necessary environment variables needed for our image 
# and build the API server.
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -ldflags="-s -w" -o apiserver .

FROM gcr.io/distroless/base-debian10

# Copy binary and config files from /build 
# to root folder of scratch container.
COPY --from=builder ["/build/apiserver", "/build/.env", "/"]

ENV IN_CONTAINER=Yes

# Export necessary port.
EXPOSE 5000

USER nonroot:nonroot

# Command to run when starting the container.
ENTRYPOINT ["/apiserver"]