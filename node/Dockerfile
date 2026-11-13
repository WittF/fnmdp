# Build context is the repo root (so go.work + shared/ are visible).
# Build: docker build -f node/Dockerfile -t fnmdp-node .
FROM golang:1.25-alpine AS build
WORKDIR /src
# Copy the workspace file and the modules it references.
COPY go.work ./
COPY shared/ ./shared/
COPY master/ ./master/
COPY node/ ./node/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/node ./node

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/node /node
ENTRYPOINT ["/node"]
