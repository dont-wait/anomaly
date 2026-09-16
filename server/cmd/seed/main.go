package main

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	seedCCCD     = "079099123456"
	seedPassword = "Demo@12345"
	seedUsername = "phamdinhminhhieu"
	seedEmail    = "phamdinhminhhieu@gmail.com"
	seedFullName = "Phạm Đình Minh Hiếu"
)

func newID() string {
	return bson.NewObjectID().Hex()
}

func main() {
	ctx := context.Background()
	log := logger.NewLogger(zerolog.InfoLevel)

	loader := domain.GetEnvLoader().Load(log)
	mongoConfig := loader.LoadMongoConfig()

	client, err := mongo.NewMongoClient(ctx, mongoConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer func() { _ = client.Disconnect(ctx) }()

	repo := mongo.NewAccountAggregateRepository(client, mongoConfig.MongoDBName)
	if err := repo.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure indexes failed")
	}

	if existing, err := repo.FindByUsername(ctx, seedUsername); err != nil {
		log.Fatal().Err(err).Msg("check existing seed failed")
	} else if existing != nil {
		fmt.Println("Seed data đã tồn tại, bỏ qua.")
		printCredentials()
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(seedPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal().Err(err).Msg("hash password failed")
	}

	now := time.Now().UTC()
	dob := time.Date(1998, 5, 20, 0, 0, 0, 0, time.UTC)
	issuedDate := time.Date(2021, 3, 15, 0, 0, 0, 0, time.UTC)

	accountID := newID()
	customerID := newID()
	kycID := newID()

	acc := &accountdomain.UserAccount{
		Id:           accountID,
		AccountNo:    "99999180105",
		CustomerId:   customerID,
		Username:     seedUsername,
		Email:        seedEmail,
		PasswordHash: string(hash),
		Type:         accountdomain.AccountTypePayment,
		Currency:     accountdomain.CurrencyVND,
		Balance:      accountdomain.Balance{Current: 128_540_000},
		Status:       accountdomain.AccountStatusActive,
		Version:      1,
		OpenedAt:     now,
		CreatedAt:    now,
		UpdatedAt:    now,
		Customer: &accountdomain.Customer{
			Id:           customerID,
			CustomerCode: fmt.Sprintf("CUS-%s", customerID),
			Profile: accountdomain.CustomerProfile{
				FullName:    seedFullName,
				DateOfBirth: &dob,
				Phone:       "0912345678",
				Email:       seedEmail,
				Address: accountdomain.Address{
					Line:         "123 Đường Lê Duẩn, Phường Bến Nghé",
					ProvinceCode: "79",
				},
			},
			Identity: accountdomain.CustomerIdentity{
				Type:             "cccd",
				Number:           seedCCCD,
				IssuedDate:       &issuedDate,
				IssuedPlace:      "Cục Cảnh sát QLHC về TTXH",
				PermanentAddress: "123 Đường Lê Duẩn, Phường Bến Nghé, TP. Hồ Chí Minh",
			},
			VerifiedKYCSessionId: kycID,
			KYCStatus:            accountdomain.KYCStatusVerified,
			CreditProfile: accountdomain.CreditProfile{
				Score:     750,
				BadDebt:   false,
				UpdatedAt: now,
			},
			Status:    accountdomain.CustomerStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
		KYCSessions: []*accountdomain.KYCSession{
			{
				Id:         kycID,
				CustomerId: customerID,
				AttemptNo:  1,
				Status:     accountdomain.KYCSessionStatusVerified,
				IdentityData: accountdomain.KYCIdentityData{
					Type:             "cccd",
					Number:           seedCCCD,
					FullName:         seedFullName,
					DateOfBirth:      &dob,
					IssuedDate:       &issuedDate,
					IssuedPlace:      "Cục Cảnh sát QLHC về TTXH",
					PermanentAddress: "123 Đường Lê Duẩn, Phường Bến Nghé, TP. Hồ Chí Minh",
				},
				Media: accountdomain.KYCMedia{
					IdentityFront: accountdomain.MediaObject{StorageKey: "seed/idcard-front.jpg"},
					IdentityBack:  accountdomain.MediaObject{StorageKey: "seed/idcard-back.jpg"},
					LivenessVideo: accountdomain.LivenessVideo{
						MediaObject: accountdomain.MediaObject{StorageKey: "seed/liveness.mp4"},
					},
				},
				Verification: accountdomain.KYCVerification{
					OCRStatus:       accountdomain.VerificationStatusNotRun,
					LivenessStatus:  accountdomain.VerificationStatusNotRun,
					FaceMatchStatus: accountdomain.VerificationStatusNotRun,
				},
				StartedAt:   now,
				CompletedAt: &now,
				CreatedAt:   now,
			},
		},
	}

	if err := repo.Create(ctx, acc); err != nil {
		log.Fatal().Err(err).Msg("seed account failed")
	}

	fmt.Println("Seed thành công!")
	printCredentials()
}

func printCredentials() {
	fmt.Println("---")
	fmt.Println("Đăng nhập thử bằng:")
	fmt.Println("  CCCD:      " + seedCCCD)
	fmt.Println("  Mật khẩu:  " + seedPassword)
	fmt.Println("---")
}
