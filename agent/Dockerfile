############################
# STEP 1 build the image for creating the executable
############################
FROM golang:1.22-alpine3.19 as builder

# Install git + SSL ca certificates + make
# Git is required for fetching the dependencies.
# Ca-certificates is required to call HTTPS endpoints.
# Make to build go application
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "arm64" ]; then ln -s /bin/run-parts  /usr/bin/run-parts; fi
RUN apk update && apk upgrade && apk add --no-cache git ca-certificates make unzip g++ && update-ca-certificates && apk --no-cache add openssl wget && rm -rf /var/cache/apk/*

# Create appuser
RUN adduser -D -g '' appuser

# Install protoc
RUN wget https://github.com/protocolbuffers/protobuf/releases/download/v3.20.1/protoc-3.20.1-linux-x86_64.zip
RUN unzip protoc-3.20.1-linux-x86_64.zip
RUN cp ./bin/protoc /usr/local/bin/protoc
RUN cp -a ./include/. /usr/local/include/

# Install GRPC golang-plugin for protoc
RUN go install github.com/golang/protobuf/protoc-gen-go@v1.5.2

WORKDIR /app

COPY agent/go.mod agent/.
COPY agent/go.sum agent/.
COPY common/go.mod common/.
COPY common/go.sum common/.
RUN cd agent && go mod download

COPY . .

# Build the binary
RUN mkdir -p /app/synthetic-heart/
RUN touch /app/synthetic-heart/.emptyfile

# Compile the binary
RUN make build-agent

RUN mv agent/bin/* /app/synthetic-heart

############################
# STEP 2 build a small image with only the executable
############################
FROM docker.io/alpine:3.19

RUN apk update

# Add curl for curl tests
RUN apk add --no-cache curl

# Import from builder.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /bin/sh /bin/sh

# Copy our static executables
COPY --from=builder --chown=appuser:appuser /app/synthetic-heart /app/synthetic-heart

# Create a /tmp/ diretctory (required for go plugin for Unix Domain Socket)
COPY --from=builder --chown=appuser:appuser /app/synthetic-heart/.emptyfile /tmp/.emptyfile


# Use an unprivileged user.
USER appuser

WORKDIR /app/synthetic-heart

# Run the hello binary.
ENTRYPOINT ["./agent"]

