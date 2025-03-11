FROM golang:latest

WORKDIR /usr/src/app

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY ./go.mod ./go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags "-linkmode 'external' -extldflags '-static'" -v -o /usr/local/bin/mesher_server ./server/server.go

FROM scratch
COPY --from=0 /usr/local/bin/mesher_server /bin/mesher_server
CMD ["mesher_server"]
