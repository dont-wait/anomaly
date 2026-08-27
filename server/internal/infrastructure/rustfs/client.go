package rustfs

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/logger"
)

// tạo kết nối với rustfs
// nhân vào cấu hình RustFSConfig và trả về client s3 (AWS SDK cung cấp sẵn)
func NewClient(conf *domain.RustFSConfig) *s3.Client {
	log := logger.NewLogger(zerolog.InfoLevel)

	cfg := aws.Config{
		Region: conf.Region,
		Credentials: aws.NewCredentialsCache(
			credentials.NewStaticCredentialsProvider(conf.AccessKey, conf.SecretKey, ""),
		),
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) { // hàm của AWS SDK, nhận cfg và trả về client s3
		o.BaseEndpoint = aws.String(conf.Endpoint) // đừng gọi amazon mà gọi địa chỉ url của rustfs
		o.UsePathStyle = true                      // rustfs cần dạng url thay vì bucket.endpoint.key
	})

	log.Info().Str("endpoint", conf.Endpoint).Msg("rustfs client initialized")

	return client
}

func EnsureBucket(ctx context.Context, client *s3.Client, bucket string) error {
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}) // xem bucket đã tồn tại ch
	if err == nil {
		return nil // bucket đã tồn tại
	}

	// chưa có thì tạo
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		var alreadyOwned *types.BucketAlreadyOwnedByYou
		var alreadyExists *types.BucketAlreadyExists
		if errors.As(err, &alreadyOwned) || errors.As(err, &alreadyExists) {
			return nil
		}
		return err
	}

	return nil
}
