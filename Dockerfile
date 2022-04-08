FROM golang:alpine as build
WORKDIR /build
COPY go.* ./
RUN go mod download
COPY . .
RUN apk add --no-cache git
RUN go build -v -o app .

FROM alpine
WORKDIR /service
COPY --from=build /build/app .
RUN apk add --no-cache tzdata
ENTRYPOINT ["./app"]