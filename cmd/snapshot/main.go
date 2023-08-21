package main

import (
	"context"
	"fmt"
	"github.com/go-logr/zapr"
	"github.com/xiaoxin1992/etcd-operator/cmd/snapshot/cos"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/snapshot"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"strings"
	"syscall"
	"time"
)

// NewUpload(*filename, *secretID, *secretKey, *region, *bucketName, context.Background())
type backup struct {
	filename   string
	secretID   string
	secretKey  string
	region     string
	bucketName string
	ctx        context.Context
	endpoints  []string
}

func (b *backup) Snapshot() {
	upload := cos.NewUpload(b.filename, b.secretID, b.secretKey, b.region, b.bucketName, b.ctx)
	zapLogger := zap.NewRaw(zap.UseDevMode(true))
	ctrl.SetLogger(zapr.NewLogger(zapLogger))
	path := filepath.Join(".", b.filename)
	logger := ctrl.Log.WithName("backup")
	config := clientv3.Config{
		Endpoints:   b.endpoints,
		DialTimeout: time.Duration(5 * time.Second),
	}
	err := snapshot.Save(b.ctx, zapLogger, config, path)
	if err != nil {
		logger.Error(err, fmt.Sprintf("backup failed %s", err.Error()))
		syscall.Exit(syscall.SYS_EXIT)
	}
	logger.Info(fmt.Sprintf("backup file success path: %s", path))
	err = upload.UploadFile()
	if err != nil {
		logger.Error(err, fmt.Sprintf("upload backup snapshot to cos failed. %s", err.Error()))
		syscall.Exit(syscall.SYS_EXIT)
	}
	logger.Info("upload backup snapshot to cos success.")

}

type restore struct {
	filename   string
	secretID   string
	secretKey  string
	region     string
	bucketName string
	ctx        context.Context
	endpoints  []string
}

func (r *restore) Snapshot() {
	zapLogger := zap.NewRaw(zap.UseDevMode(true))
	ctrl.SetLogger(zapr.NewLogger(zapLogger))
	logger := ctrl.Log.WithName("restore")
	upload := cos.NewUpload(r.filename, r.secretID, r.secretKey, r.region, r.bucketName, r.ctx)
	err := upload.Download()
	if err != nil {
		logger.Error(err, fmt.Sprintf("download snapshot db failed. %s", err.Error()))
		syscall.Exit(syscall.SYS_EXIT)
	}
	logger.Info("download snapshot db success.")
}

func main() {
	// OPTIONTYPE   备份或者恢复{backup, restore}
	// FILENAME     备份文件名,恢复远程文件的文件名
	// AK           腾讯云的secretID
	// SK           腾讯云的secretKey
	// REGION       cos所在的区域
	// BUCKET        bucket名称
	// ENDPOINTS     etcd 连接地址
	optType := os.Getenv("OPTIONTYPE")
	fileName := os.Getenv("FILENAME")
	secretID := os.Getenv("AK")
	secretKey := os.Getenv("SK")
	region := os.Getenv("REGION")
	bucketName := os.Getenv("BUCKET")
	endpoints := os.Getenv("ENDPOINTS")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if optType == "backup" {
		b := backup{
			filename:   fileName,
			secretID:   secretID,
			secretKey:  secretKey,
			region:     region,
			bucketName: bucketName,
			ctx:        ctx,
			endpoints:  strings.Split(endpoints, ","),
		}
		b.Snapshot()
	}
	if optType == "restore" {
		b := restore{
			filename:   fileName,
			secretID:   secretID,
			secretKey:  secretKey,
			region:     region,
			bucketName: bucketName,
			ctx:        ctx,
			endpoints:  strings.Split(endpoints, ","),
		}
		b.Snapshot()
	}
}

//export OPTIONTYPE="backup"
//export FILENAME="etcd-snapshot.db"
//export AK="AKIDrXAYotqJrGWaYJxdO6nQrnkY7QYUFWkM"
//export SK="hP0a5vpttNrAHSYUE5qOl7VwNhqzxmU9"
//export REGION="ap-beijing"
//export BUCKET="etcd-1308765018"
//export ENDPOINTS="127.0.0.1:2379"

//export OPTIONTYPE="restore"
//export FILENAME="etcd-snapshot.db"
//export AK="AKIDrXAYotqJrGWaYJxdO6nQrnkY7QYUFWkM"
//export SK="hP0a5vpttNrAHSYUE5qOl7VwNhqzxmU9"
//export REGION="ap-beijing"
//export BUCKET="etcd-1308765018"
//export ENDPOINTS="127.0.0.1:2379"
