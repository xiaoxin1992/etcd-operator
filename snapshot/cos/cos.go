package cos

import (
	"context"
	"fmt"
	"github.com/tencentyun/cos-go-sdk-v5"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type progress struct{}

func (p *progress) ProgressChangedCallback(event *cos.ProgressEvent) {
	switch event.EventType {
	case cos.ProgressStartedEvent:
		fmt.Println("start transfer file")
	case cos.ProgressDataEvent:
		fmt.Printf("\rtransfer progress %d%%", event.ConsumedBytes*100/event.TotalBytes)
	case cos.ProgressFailedEvent:
		fmt.Printf("transfer failed: %v\n", event.Err)
	case cos.ProgressCompletedEvent:
		fmt.Println("\ntransfer file completed.")

	}
}

func NewUpload(filepath, secretID, secretKey, region, bucketName string, ctx context.Context) *Upload {
	return &Upload{
		filepath:   filepath,
		secretID:   secretID,
		secretKey:  secretKey,
		region:     region,
		bucketName: bucketName,
		ctx:        ctx,
	}
}

type Upload struct {
	filepath   string
	secretID   string
	secretKey  string
	region     string
	bucketName string
	client     *cos.Client
	ctx        context.Context
}

func (u *Upload) checkFile() (err error) {
	u.filepath, err = filepath.Abs(u.filepath)
	if err != nil {
		return
	}
	stat, err := os.Stat(u.filepath)
	if err != nil && os.IsNotExist(err) {
		return
	}
	if stat.IsDir() {
		err = fmt.Errorf("%s is dir", u.filepath)
		return
	}
	return
}

func (u *Upload) UploadFile() (err error) {
	u.init()
	err = u.checkFile()
	if err != nil {
		return
	}
	fd, err := os.Open(u.filepath)
	if err != nil {
		return
	}
	defer fd.Close()
	defer fd.Close()
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: "text/html",
			Listener:    &progress{},
		},
	}
	_, err = u.client.Object.Put(u.ctx, filepath.Base(u.filepath), fd, opt)
	return
}
func (u *Upload) Download() (err error) {
	u.init()
	localPath, err := filepath.Abs(".")
	if err != nil {
		return
	}
	s, err := os.Stat(localPath)
	if err != nil {
		return
	}
	if s.IsDir() {
		localPath = filepath.Join(localPath, u.filepath)
	}
	opt := &cos.ObjectGetOptions{
		ResponseContentType: "text/html",
		// 使用默认方式查看进度
		Listener: &progress{},
	}
	_, err = u.client.Object.GetToFile(context.Background(), u.filepath, localPath, opt)
	if err == nil {
		fmt.Printf("download file save to %s\n", localPath)
	}
	return
}

func (u *Upload) init() {
	bucketURL, _ := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", u.bucketName, u.region))
	serviceURL, _ := url.Parse(fmt.Sprintf("https://cos.%s.myqcloud.com", u.region))
	baseURL := &cos.BaseURL{BucketURL: bucketURL, ServiceURL: serviceURL}
	client := cos.NewClient(baseURL, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  u.secretID,
			SecretKey: u.secretKey,
		},
	})
	u.client = client
	return
}
