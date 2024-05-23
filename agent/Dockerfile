############################
# STEP 1 build the image for creating the executable
############################
FROM docker.io/library/golang:1.22-alpine3.19 as builder

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

COPY agent/go.mod agent/.
COPY agent/go.sum agent/.
COPY common/go.mod common/.
COPY common/go.sum common/.
RUN cd agent && go mod download

COPY . .

# Build the binary
RUN mkdir -p /app/synthetic-heart/

# Compile the binary
RUN cd agent/ && make build-go-syntest-plugins

RUN mv agent/bin/* /app/synthetic-heart

############################
# STEP 2 build a small image with only the executables
############################
FROM localhost/synheart-agent:dev-latest-no-plugins

# Copy our static executables
COPY --from=builder /app/synthetic-heart/plugins/ /app/synthetic-heart/plugins/

# Install CURL for syntest
RUN apk add curl