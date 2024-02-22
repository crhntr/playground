FROM golang:alpine
COPY . /playground
WORKDIR /playground
RUN cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" cmd/server/assets/
RUN go build -o app ./cmd/server
CMD ["/playground/app"]