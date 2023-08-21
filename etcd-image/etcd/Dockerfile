FROM golang:1.20 as builder
WORKDIR /opt
RUN export GOPROXY="https://proxy.golang.com.cn,direct"
RUN git clone -b v3.5.0 https://github.com/etcd-io/etcd.git
RUN cd etcd && ./build.sh && ls . && ls ./bin
RUN mkdir /etcd && cp -r /opt/etcd/bin /etcd

FROM ubuntu:20.04
WORKDIR /
COPY --from=builder /etcd/bin ./bin
COPY ENTRYPOINT PRESTOP check_status ./opt/
RUN  sed -i s@/archive.ubuntu.com/@/mirrors.aliyun.com/@g /etc/apt/sources.list && sed -i s@/security.ubuntu.com/@/mirrors.aliyun.com/@g /etc/apt/sources.list && apt-get clean && apt-get update -y && apt-get install iputils-ping -y && apt-get clean && chmod +x ./opt/ENTRYPOINT && chmod +x ./opt/check_status && chmod +x ./opt/PRESTOP
CMD [ "./opt/ENTRYPOINT" ]