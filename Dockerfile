# syntax=docker/dockerfile:1

# Go: the WASM sim for the client and the native server, from the same source.
FROM golang:1.26.5 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY sim sim
COPY data data
COPY server server
RUN GOOS=js GOARCH=wasm go build -o /out/main.wasm ./sim/wasm \
 && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" /out/ \
 && CGO_ENABLED=0 go build -o /out/server ./server

# Client: the same steps as `pnpm build`, with the WASM taken from the Go stage.
FROM node:24-slim AS client
RUN npm install -g pnpm@11
WORKDIR /client
COPY client/package.json client/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY client ./
COPY --from=go /out/main.wasm /out/wasm_exec.js public/
RUN pnpm exec tsc -b && pnpm exec vite build

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /out/server /server
COPY --from=client /client/dist /client
EXPOSE 8080
ENTRYPOINT ["/server", "-static", "/client"]
