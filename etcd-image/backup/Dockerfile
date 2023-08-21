FROM golang:1.20 as builder
WORKDIR /opt
RUN export GOPROXY="https://proxy.golang.com.cn,direct"
COPY ../..  /opt
#RUN go build -o snapshot /opt/snapshot/main.go

#FROM ubuntu:20.04
#WORKDIR /opt
#COPY --from=builder snapshot /opt
#COPY ENTRYPOINT ./opt/
#CMD [ "./opt/ENTRYPOINT" ]