// Run in MongoDB Compass -> MONGOSH with:
// load("E:/KhoaLuan/Project/anomaly/server/scripts/mongosh/seed-savings-terms.js")
// This is a one-time idempotent bootstrap, not a live API fallback.
(() => {
const savingsDB = db.getSiblingDB("anomaly");
const savingsTerms = savingsDB.getCollection("savings_terms");
// Report duplicates with concrete product codes before attempting a unique index.
const duplicates = savingsTerms.aggregate([
  { $match: { status: "ACTIVE", product_code: { $exists: true } } },
  { $group: { _id: "$product_code", count: { $sum: 1 }, ids: { $push: "$_id" } } },
  { $match: { count: { $gt: 1 } } }
]).toArray();
if (duplicates.length) {
  printjson({ error: "Resolve duplicate ACTIVE savings terms before indexing", duplicates });
  throw new Error("Duplicate active savings product terms; no data has been modified");
}
savingsTerms.createIndex(
  { product_code: 1, status: 1 },
  { name: "uniq_active_savings_terms", unique: true,
    partialFilterExpression: { status: "ACTIVE", product_code: { $exists: true } } }
);
const savingsSeed = {
  "_id": "savings-standard-v1",
  "product_code": "savings_standard",
  "status": "ACTIVE",
  "version": "savings-standard-v1",
  "title": "Điều kiện gửi tiết kiệm có kỳ hạn",
  "introduction": "Các điều kiện dưới đây áp dụng cho sản phẩm Tiết kiệm có kỳ hạn bằng VND tại AnomalyBank. Số tiền gửi, kỳ hạn, lãi suất và ngày đáo hạn của từng sổ được thể hiện tại bước xác nhận và ghi nhận trong thông tin hợp đồng sau khi mở sổ thành công.",
  "sections": [
    {
      "title": "Điều kiện mở sổ",
      "body": "Khách hàng cần có tài khoản thanh toán VND đang hoạt động và đã hoàn tất xác thực. Số tiền gửi tối thiểu là 1.000.000 VND mỗi sổ. Tài khoản trích tiền phải thuộc sở hữu khách hàng và có đủ số dư khả dụng tại thời điểm xác nhận."
    },
    {
      "title": "Kỳ hạn và lãi suất",
      "body": "Các kỳ hạn áp dụng gồm 3, 6, 9, 12, 24, 36 và 60 tháng. Lãi suất của sổ được cố định tại thời điểm mở thành công. Tiền lãi được tính theo lãi đơn, số ngày gửi thực tế trên cơ sở 365 ngày/năm và làm tròn đến đồng Việt Nam một lần."
    },
    {
      "title": "Xác nhận mở sổ",
      "body": "Thông tin dự tính tiền gửi có hiệu lực trong 5 phút. Sau khi khách hàng kiểm tra và xác nhận, hệ thống sẽ kiểm tra lại điều kiện mở sổ, trích số tiền gửi từ tài khoản thanh toán và ghi nhận hợp đồng tiết kiệm khi giao dịch hoàn tất."
    },
    {
      "title": "Đáo hạn và tất toán",
      "body": "Sổ tiết kiệm được tất toán toàn bộ theo yêu cầu của khách hàng kể từ thời điểm đáo hạn. Gốc và lãi được chuyển về tài khoản thanh toán đã dùng để mở sổ. Không áp dụng rút trước hạn, rút một phần, gửi bổ sung hoặc tự động tái tục. Tiền lãi không tăng thêm sau thời điểm đáo hạn."
    },
    {
      "title": "Tra cứu thông tin",
      "body": "Khách hàng có thể tra cứu danh sách sổ, thông tin hợp đồng, trạng thái và lịch sử giao dịch gửi hoặc tất toán trong mục Tiết kiệm của ứng dụng."
    }
  ]
};
const savingsResult = savingsTerms.updateOne(
  { _id: savingsSeed._id },
  { $setOnInsert: savingsSeed },
  { upsert: true }
);
printjson({ matchedCount: savingsResult.matchedCount, upsertedCount: savingsResult.upsertedCount });
printjson(savingsTerms.findOne({ _id: savingsSeed._id }));
})();
