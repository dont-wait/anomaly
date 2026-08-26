package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// checkpointID định danh duy nhất cho checkpoint của projection worker
// trên stream $all. Hiện chỉ chạy 1 worker nên dùng 1 khóa cố định;
// nếu sau này chạy nhiều worker song song, cần đổi thành khóa theo
// từng consumer group.
const checkpointID = "projection-worker-all"

type checkpointRecord struct {
	Id      string `bson:"_id"`
	Commit  uint64 `bson:"commit"`
	Prepare uint64 `bson:"prepare"`
}

// CheckpointRepository lưu vị trí (Commit/Prepare) trong stream $all mà
// worker đã xử lý xong lần gần nhất. Khi worker restart, đọc lại vị trí
// này để subscribe tiếp đúng chỗ đã dừng, thay vì phải replay lại toàn
// bộ lịch sử event (tốn thời gian) hoặc bỏ sót event phát sinh lúc
// worker đang down (mất dữ liệu projection).
type CheckpointRepository struct {
	col *mongodrv.Collection
}

func NewCheckpointRepository(client *mongodrv.Client, dbName string) *CheckpointRepository {
	return &CheckpointRepository{col: client.Database(dbName).Collection("checkpoints")}
}

// Load trả về (commit, prepare, found, error).
// found=false nghĩa là chưa từng lưu checkpoint (lần chạy đầu tiên của
// worker) -> nên subscribe từ đầu ($all Start).
func (r *CheckpointRepository) Load(ctx context.Context) (commit, prepare uint64, found bool, err error) {
	var rec checkpointRecord
	err = r.col.FindOne(ctx, bson.M{"_id": checkpointID}).Decode(&rec)
	if err != nil {
		if err == mongodrv.ErrNoDocuments {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	return rec.Commit, rec.Prepare, true, nil
}

// Save upsert vị trí mới nhất đã xử lý xong. Được gọi sau MỖI event
// nhận từ $all (kể cả event bị bỏ qua vì không liên quan tới account),
// để checkpoint luôn tiến lên, tránh worker phải đọc lại các event
// không liên quan mỗi lần restart.
func (r *CheckpointRepository) Save(ctx context.Context, commit, prepare uint64) error {
	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": checkpointID},
		bson.M{"$set": bson.M{"commit": commit, "prepare": prepare}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
