package util

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	sysv1 "bytetrade.io/web3os/backup-server/pkg/apis/sys.bytetrade.io/v1"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	pkgerrors "github.com/pkg/errors"
)

var defaultDelimiter = "/"

type UploadResult struct {
	Size           int64
	Location       string
	UploadID       string
	CompletedParts int
	ETag           *string
	Key            *string
}

type S3Option struct {
	Region    string
	S3URL     string
	Bucket    string
	Prefix    string
	AccessKey string
	SecretKey string
}

type S3 struct {
	o *sysv1.BackupConfigSpec

	cfg aws.Config

	client *s3.Client

	uploader *manager.Uploader

	delimiter string
}

func NewS3Client(o *sysv1.BackupConfigSpec) (*S3, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(o.Region),
	)
	if err != nil {
		return nil, pkgerrors.WithStack(err)
	}

	if o.S3Url != "" {
		cfg.EndpointResolverWithOptions = aws.EndpointResolverWithOptionsFunc(func(service, region string,
			options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               o.S3Url,
				SigningRegion:     o.Region,
				HostnameImmutable: true,
			}, nil
		})
	}

	// new s3 client with env credential
	client := s3.NewFromConfig(cfg)

	s := &S3{
		cfg:       cfg,
		o:         o,
		client:    client,
		delimiter: defaultDelimiter,
	}

	s.uploader = manager.NewUploader(client)
	s.uploader.PartSize = 10 << 20

	return s, nil
}

func (s *S3) SetDelimiter(delimiter string) {
	s.delimiter = delimiter
}

func (s *S3) ListBuckets(ctx context.Context) (*s3.ListBucketsOutput, error) {
	return s.client.ListBuckets(ctx, &s3.ListBucketsInput{})
}

func (s *S3) ListSubDirs(ctx context.Context, prefix string) ([]string, error) {
	objects, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.o.Bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String(s.delimiter),
	})

	if err != nil {
		return nil, pkgerrors.WithStack(err)
	}

	res := []string(nil)

	for _, each := range objects.CommonPrefixes {
		res = append(res, *each.Prefix)
	}
	return res, nil
}

// Deprecated: UploadFile is deprecated.
func (s *S3) UploadFile(ctx context.Context, fileName, uploadSourceFilePath string, expires *time.Time) (*UploadResult, error) {
	f, err := os.Open(uploadSourceFilePath)
	if err != nil {
		return nil, pkgerrors.WithStack(err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, pkgerrors.WithStack(err)
	}

	r, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		ContentLength: fi.Size(),
		Bucket:        aws.String(s.o.Bucket),
		Body:          f,
		Expires:       expires,
	})
	if err != nil {
		return nil, pkgerrors.WithStack(err)
	}

	return &UploadResult{
		Size:           fi.Size(),
		Location:       r.Location,
		UploadID:       r.UploadID,
		CompletedParts: len(r.CompletedParts),
		ETag:           r.ETag,
		Key:            r.Key,
	}, nil
}

// Deprecated: DownloadFile is deprecated.
func (s *S3) DownloadFile(ctx context.Context, fileName, downloadToFilePath string) (int64, error) {
	dirName := filepath.Dir(downloadToFilePath)
	if dirName != "" {
		err := os.MkdirAll(dirName, 0755)
		if err != nil {
			return 0, pkgerrors.WithStack(err)
		}
	}

	w, err := os.OpenFile(downloadToFilePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return 0, pkgerrors.WithStack(err)
	}
	defer w.Close()

	keyName := fileName

	bs, err := manager.NewDownloader(s.client).Download(ctx, w, &s3.GetObjectInput{
		Bucket: aws.String(s.o.Bucket),
		Key:    aws.String(keyName),
	})
	if err != nil {
		return 0, pkgerrors.WithStack(err)
	}
	return bs, nil
}

func (s *S3) DeleteObject(ctx context.Context, key string) error {
	if strings.HasSuffix(key, "/") {
		listOutput, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(s.o.Bucket),
			Prefix: aws.String(key),
		})
		if err != nil {
			return pkgerrors.WithStack(err)
		}

		for _, object := range listOutput.Contents {
			_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(s.o.Bucket),
				Key:    object.Key,
			})
			if err != nil {
				log.Errorf("delete object %q, %v", object.Key, err)
				//return pkgerrors.WithStack(err)
			}
		}
	}

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.o.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return pkgerrors.WithStack(err)
	}
	return nil
}

func (s *S3) GetObject(ctx context.Context, key string) (*s3.GetObjectOutput, error) {
	res, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.o.Bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, pkgerrors.WithStack(err)
	}
	return res, nil
}

func (s *S3) ObjectExists(ctx context.Context, key string) (bool, error) {
	res, err := s.GetObject(ctx, key)

	if err != nil {
		var nke *types.NoSuchKey
		if errors.As(err, &nke) {
			return false, nil
		}
		return false, err
	}

	return res != nil, nil
}
