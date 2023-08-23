FROM golang:1.20 as builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /opt
RUN export GOPROXY="https://proxy.golang.com.cn,direct"
RUN git clone https://github.com/xiaoxin1992/etcd-operator.git
RUN cd etcd-operator && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o snap snapshot/main.go && ls

FROM alpine:3.18.3
WORKDIR /opt
COPY --from=builder /opt/etcd-operator/snap ./backup
COPY ENTRYPOINT ./
CMD ["/opt/ENTRYPOINT" ]

