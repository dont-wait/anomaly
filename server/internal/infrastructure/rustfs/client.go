package rustfs

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/logger"
)

// tạo kết nối với rustfs
// nhân vào cấu hình RustFSConfig và trả về client s3 (AWS SDK cung cấp sẵn)
func NewClient(conf *domain.RustFSConfig) *s3.Client {
	log := logger.NewLogger(zerolog.InfoLevel)

	cfg := aws.Config{ // cấu hình AWS SDK
		Region: "us-east-1",
		Credentials: aws.NewCredentialsCache( // bọc lại credentials provider
			credentials.NewStaticCredentialsProvider(conf.AccessKey, conf.SecretKey, ""), // nhận access key và secret key
		),
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) { // hàm của AWS SDK, nhận cfg và trả về client s3
		o.BaseEndpoint = aws.String(conf.Endpoint) // đừng gọi amazon mà gọi địa chỉ url của rustfs
		o.UsePathStyle = true                      // rustfs cần dạng url thay vì bucket.endpoint.key
	})

	log.Info().Str("endpoint", conf.Endpoint).Msg("rustfs client initialized")

	return client
}
