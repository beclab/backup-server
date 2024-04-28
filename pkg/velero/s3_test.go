package velero

import (
	"context"
	"fmt"
	"testing"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
)

var minioBcSpec = &sysv1.BackupConfigSpec{
	Provider:  "minio",
	Bucket:    "terminus",
	S3Url:     "http://192.168.50.35:9000",
	AccessKey: "minioadmin",
	SecretKey: "p7poo5dksqm84d4r",
}

var s3Client *S3

func init() {
	log.InitLog("debug")

	var err error
	var spec = minioBcSpec

	SetDefaultBackupConfigSpec(spec)
	fmt.Printf("bc: %s\n\n", util.PrettyJSON(spec))

	s3Client, err = NewS3Client(spec)
	if err != nil {
		panic(err)
	}
}

func TestListBuckets(t *testing.T) {
	buckets, err := s3Client.ListBuckets(context.TODO())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("buckets:\n %s\n", util.PrettyJSON(buckets.Buckets))
}

func TestUploadFile(t *testing.T) {
	expires := time.Now().Add(time.Duration(DefaultBackupTTL) * 24 * time.Hour)
	r, err := s3Client.UploadFile(context.TODO(), "test.50m", "/Users/zl/test.50m", &expires)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("upload result:\n%s\n", util.PrettyJSON(r))
}

func TestDownloadFile(t *testing.T) {
	gotBytes, err := s3Client.DownloadFile(context.TODO(), "test.50m",
		"/tmp/test.50m")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("download file %d bytes", gotBytes)
}

func TestListSubDirs(t *testing.T) {
	dirs, err := s3Client.ListSubDirs(context.TODO(), "test")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("list sub dirs:")
	for _, dir := range dirs {
		fmt.Printf("%s\n", dir)
	}
}

func TestDeleteFile(t *testing.T) {
	if err := s3Client.DeleteObject(
		context.TODO(), "zl-081805.tar.gz"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteDir(t *testing.T) {
	err := s3Client.DeleteObject(
		context.TODO(), "/pprof/")
	if err != nil {
		t.Fatal(err)
	}
}

func TestObjectExists(t *testing.T) {
	exists, err := s3Client.ObjectExists(
		context.TODO(), "/test")
	if err != nil {
		t.Fatal(err)
	}

	if !exists {
		t.Logf("object test-dify.txt not exists")
	}
}
