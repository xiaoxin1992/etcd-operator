#! /bin/bash
docker buildx create --name mybuilder
docker buildx use mybuilder
docker buildx build --platform linux/arm64,linux/amd64  -t joinlulu/etcd-operator:v3.5.0  . --push
docker buildx  rm mybuilder
