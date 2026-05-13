FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/production-decision-server ./examples/production_decision_server

FROM alpine:3.22
RUN adduser -D -H app && mkdir -p /data && chown -R app:app /data
WORKDIR /app
COPY --from=build /out/production-decision-server /app/production-decision-server
COPY examples/decision_intelligence/packages/fraud.bcl /app/fraud.bcl
COPY examples/production_decision_server/production.authz /app/production.authz
USER app
ENV ADDR=:8080
ENV DECISION_SQLITE_PATH=/data/decisions.db
ENV DECISION_PACKAGE=/app/fraud.bcl
ENV DECISION_AUTHZ_FILE=/app/production.authz
EXPOSE 8080
ENTRYPOINT ["/app/production-decision-server"]
