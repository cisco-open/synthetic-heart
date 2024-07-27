############################
# STEP 1 build the image for creating the executable
############################
FROM docker.io/library/golang:1.22.5-alpine3.19 as builder

# Install git + SSL ca certificates + make
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

RUN mkdir agent
COPY agent/go.mod agent/.
COPY agent/go.sum agent/.
RUN mkdir common
COPY common/go.mod common/.
COPY common/go.sum common/.
RUN cd agent && go mod download

COPY . .

# Build the binary
RUN mkdir -p /app/synthetic-heart/
RUN touch /app/synthetic-heart/.emptyfile

# Compile the binary
RUN cd agent/ && make build-agent

# Move everything to the final location
RUN mv agent/bin/* /app/synthetic-heart
RUN mv agent/plugins/syntests-python/ /app/synthetic-heart/plugins-python


############################
# STEP 2 build a small image with only the executable
############################
FROM docker.io/library/alpine:3.19 as base

# Copy over just the agent binary
COPY --from=builder /app/synthetic-heart/agent /app/synthetic-heart/agent

# Create a /tmp/ diretctory (required for go plugin for Unix Domain Socket)
COPY --from=builder /app/synthetic-heart/.emptyfile /tmp/.emptyfile

# Fix for CVE-2024-5535
RUN apk add "openssl>3.1.6-r0"

WORKDIR /app/synthetic-heart

# Run the binary.
ENTRYPOINT ["./agent"]

############################
# STEP 3 build image with go plugins
############################
From base as base-with-go-plugins

# Copy over the go plugins
COPY --from=builder /app/synthetic-heart/plugins/* /app/synthetic-heart/plugins/

# Install CURL for syntest
RUN apk add curl

############################
# STEP 4 build image with python plugins
############################
FROM base-with-go-plugins as base-with-python-plugins

# Copy over python plugins
COPY --from=builder /app/synthetic-heart/plugins-python /app/synthetic-heart/plugins-python

# Install Python
ENV PYTHONUNBUFFERED=1
RUN apk add --update --no-cache python3 py3-pip git
RUN pip3 install --no-cache --upgrade pip setuptools --break-system-packages
RUN cd /app/synthetic-heart/plugins-python && sh install_requirements.sh --break-system-packages