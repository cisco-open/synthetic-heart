############################
# STEP 1 build the image for creating the executable
############################
FROM docker.io/library/golang:1.23.1-alpine3.20 as builder

# Install git + SSL ca certificates + make
# Git is required for fetching the dependencies.
# Ca-certificates is required to call HTTPS endpoints.
# Make to build go application
RUN apk update && apk upgrade && apk add --no-cache git ca-certificates make unzip g++ && update-ca-certificates && apk --no-cache add openssl wget && rm -rf /var/cache/apk/*

# Create appuser
RUN adduser -D -g '' appuser

WORKDIR /app
COPY . .

# Compile the binary
RUN cd restapi && make build-restapi

############################
# STEP 2 build a small image with only the executable
############################
FROM gcr.io/distroless/static:nonroot
WORKDIR /app/
COPY --from=builder /app/restapi/bin/restapi /app/restapi
USER 65532:65532
ENTRYPOINT ["./restapi"]

