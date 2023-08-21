FROM golang:1.20 as builder
WORKDIR /opt
RUN export GOPROXY="https://proxy.golang.com.cn,direct"
RUN git clone https://github.com/xiaoxin1992/etcd-operator.git
RUN cd etcd-operator && go build -o snapshot cmd/snapshot/main.go && ls

FROM ubuntu:20.04
WORKDIR /opt
COPY --from=builder /opt/etcd-operator/snapshot /opt
COPY ENTRYPOINT ./opt/
CMD [ "./opt/ENTRYPOINT" ]