# Web viewer: React UI + Go server in one small image.
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npx tsc -b && npx vite build --outDir /dist --emptyOutDir

FROM golang:1.26-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server . \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hashpw ./cmd/hashpw

FROM alpine:3.21
RUN apk add --no-cache ffmpeg ca-certificates tzdata \
    && adduser -D -H app && mkdir /data && chown app /data
COPY --from=server /out/server /out/hashpw /usr/local/bin/
USER app
ENV ADDR=:8080 DATA_DIR=/data
VOLUME /data
EXPOSE 8080
CMD ["server"]
