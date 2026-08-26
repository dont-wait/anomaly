package rustfs

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// MediaRepository lưu và lấy media (ảnh CCCD, ảnh khuôn mặt...) trên
// RustFS. "key" là đường dẫn/tên định danh duy nhất của file trong
// bucket, ví dụ: "cccd/{accountID}/front.jpg". không cần chuyền bucket vì
type MediaRepository struct {
	client *s3.Client
	bucket string
}

func NewMediaRepository(client *s3.Client, bucket string) *MediaRepository {
	return &MediaRepository{client: client, bucket: bucket}
}

// Upload đẩy 1 file lên RustFS tại đúng "key" chỉ định. contentType
// nên truyền đúng MIME type thật (vd "image/jpeg") để khi client tải
// lại file, trình duyệt/app hiển thị đúng, không bị coi là file tải
// xuống chung chung.
func (r *MediaRepository) Upload(
	ctx context.Context, // context để cancel nếu cần
	key string, // tên định danh file
	body io.Reader, // nội dung file
	contentType string, // loại file
) error {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{ // thực hiện gửi requets lên rustfs
		Bucket:      &r.bucket,
		Key:         &key,
		Body:        body,
		ContentType: &contentType,
	})
	return err
}

// Download lấy file về từ RustFS theo "key". Caller chịu trách nhiệm
// Close() ReadCloser trả về sau khi dùng xong, để tránh leak
// connection.
// io.ReadCloser: đọc được và  cần đóng lại sau khi xài xong
func (r *MediaRepository) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{ // gưi request GET lên rustfs, lấy file theo key
		Bucket: &r.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// Delete xoá 1 file khỏi RustFS theo "key".
func (r *MediaRepository) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &r.bucket,
		Key:    &key,
	})
	return err
}
