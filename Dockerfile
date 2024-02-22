FROM golang:alpine
COPY . /playground
WORKDIR /playground
RUN ./README init
RUN ./README build_webapp
RUN go build -o app ./cmd/server
CMD ["/playground/app"]