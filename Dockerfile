FROM golang:1.25-alpine
COPY . /playground
WORKDIR /playground
RUN mkdir -p cmd/server/assets/lib && \
    cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" cmd/server/assets/lib/
RUN go build -o app ./cmd/server
CMD ["/playground/app"]