FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/decision-server ./examples/bcl_decision_platform/server

FROM alpine:3.22
WORKDIR /app
COPY --from=build /out/decision-server /app/decision-server
COPY examples/bcl_decision_platform/fraud.bcl /app/fraud.bcl
ENV ADDR=:8080
ENV DECISION_PACKAGE=/app/fraud.bcl
EXPOSE 8080
ENTRYPOINT ["/app/decision-server"]
