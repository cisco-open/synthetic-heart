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
RUN apk update && apk add --no-cache git ca-certificates make unzip g++ && update-ca-certificates

# Create appuser
RUN adduser -D -g '' appuser

WORKDIR /app
COPY . .

# Compile the binary
RUN make build-restapi

############################
# STEP 2 build a small image with only the executable
############################
FROM gcr.io/distroless/static:nonroot
WORKDIR /app/
COPY --from=builder /app/restapi/bin/restapi /app/restapi
USER 65532:65532
ENTRYPOINT ["./restapi"]

