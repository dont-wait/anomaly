package main

import (
	"context"
	netHTTP "net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/helpers"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/kyc"
	"github.com/dont-wait/anomaly/internal/infrastructure/loanadapter"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	rustfs "github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/logger"
	presentation "github.com/dont-wait/anomaly/internal/presentation/http"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/rs/zerolog"
)

func main() {
	ctx := context.Background()

	logger := logger.NewLogger(zerolog.InfoLevel)

	loader := domain.GetEnvLoader().Load(logger)
	config := loader.LoadAllConfig()
	if strings.TrimSpace(config.KafkaConfig.RiskDecisionSigningKey) == "" {
		logger.Fatal().Msg("KAFKA_RISK_DECISION_SIGNING_KEY is required")
	}

	mongoClient, err := mongo.NewMongoClient(ctx, config.MongoConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer func() {
		if err := mongoClient.Disconnect(ctx); err != nil {
			logger.Error().Err(err).Msg("disconnect mongo failed")
		}
	}()

	rustfsClient := rustfs.NewClient(config.RustFSConfig)
	if err := rustfs.EnsureBucket(ctx, rustfsClient, config.RustFSConfig.Bucket); err != nil {
		logger.Fatal().Err(err).Msg("ensure rustfs bucket failed")
	}
	mediaRepo := rustfs.NewMediaRepository(rustfsClient, config.RustFSConfig.Bucket)
	recipientContacts := mongo.NewRecipientContactRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := recipientContacts.EnsureIndexes(ctx); err != nil {
		logger.Fatal().Err(err).Msg("ensure recipient contact indexes failed")
	}
	receiptOutbox := mongo.NewReceiptOutboxRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := receiptOutbox.EnsureIndexes(ctx); err != nil {
		logger.Fatal().Err(err).Msg("ensure receipt outbox indexes failed")
	}
	riskDecisions := mongo.NewRiskDecisionRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := riskDecisions.EnsureIndexes(ctx); err != nil {
		logger.Fatal().Err(err).Msg("ensure risk decision indexes failed")
	}
	riskAssessments := mongo.NewRiskAssessmentRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := riskAssessments.EnsureIndexes(ctx); err != nil {
		logger.Fatal().Err(err).Msg("ensure risk assessment indexes failed")
	}

	esClient, err := eventstore.NewEventStoreClient(config.EventStoreConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect eventstore failed")
	}
	defer eventstore.Disconnect(esClient)
	bank := eventstore.NewBankRepository(esClient, mongo.NewTransferRepository(mongoClient, config.MongoConfig.MongoDBName))
	legacyAccounts, err := mongoClient.Database(config.MongoConfig.MongoDBName).Collection("accounts").EstimatedDocumentCount(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("check existing account data")
	}
	if err := bank.EnsureReady(ctx, legacyAccounts > 0); err != nil {
		logger.Fatal().Err(err).Msg("banking source of truth unavailable")
	}

	tokenSvc := auth.NewTokenService(config.AuthConfig.JWTSecret, config.AuthConfig.JWTExpiry)

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisConfig.Addr,
		Password: config.RedisConfig.Password,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Fatal().Err(err).Msg("connect redis failed")
	}
	logger.Info().Str("addr", config.RedisConfig.Addr).Msg("connected to redis")
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error().Err(err).Msg("disconnect redis failed")
		}
	}()

	accountHandler := composition.NewAccountHandlerWithKYC(bank, tokenSvc, kyc.NewClient(config.KYCConfig.ServiceURL, config.KYCConfig.ServiceToken), mediaRepo, *logger)
	accountHandler.WithRealtime(&kyc.RealtimeStore{DB: rdb, Policy: kyc.Policy{
		MaxAttempts: config.KYCConfig.MaxAttempts, Window: config.KYCConfig.AttemptWindow, AttemptTTL: config.KYCConfig.AttemptTTL,
		MaxConcurrent: config.KYCConfig.MaxConcurrentAttempts, StartsPerMinute: config.KYCConfig.StartsPerMinute,
	}}, helpers.SplitCSV(loader.LoadEnvOr(
		"CORS_ALLOWED_ORIGINS", "http://localhost:1420,http://localhost:1422,http://localhost:5173,http://localhost:3000,tauri://localhost,http://tauri.localhost,https://tauri.localhost")))
	go accountHandler.RunKYCStorageCleanup(ctx)
	mediaHandler := composition.NewMediaHandler(mediaRepo, bank, *logger)
	otpHandler := composition.NewOTPHandler(rdb, config.SMTPConfig, *logger)
	transactionHandler := composition.NewTransactionHandler(bank, recipientContacts, rdb, config.SMTPConfig, *logger, receiptOutbox)
	if err := transactionHandler.SetTrustedProxyCIDRs(config.HTTPConfig.TrustedProxyCIDRs); err != nil {
		logger.Fatal().Err(err).Msg("invalid trusted proxy configuration")
	}
	transactionHandler.SetRiskDecisionStore(riskDecisions)
	go composition.NewReceiptWorker(receiptOutbox, config.SMTPConfig, *logger).Run(ctx)
	go runRiskDecisionConsumer(ctx, config.KafkaConfig, riskDecisions, riskAssessments, *logger)

	// Loan Application Repository - EventStoreDB.
	loanRepo := eventstore.NewLoanRepository(esClient)

	// Loan Query Read Model - MongoDB.
	loanRead := mongo.NewLoanReadModel(
		mongoClient,
		config.MongoConfig.MongoDBName,
	)

	if err := loanRead.EnsureIndexes(ctx); err != nil {
		logger.Fatal().
			Err(err).
			Msg("ensure loan projection indexes")
	}

	// Evidence do nhân viên xác nhận.
	loanEvidence := loanadapter.NewEvidenceStore(
		mongoClient,
		config.MongoConfig.MongoDBName,
	)

	// Dependency Injection.
	loanHandler := composition.NewLoanHandler(
		bank,
		loanRepo,
		loanRead,
		loanEvidence,
		mediaRepo,
	)

	packageCatalog := mongo.NewLoanPackageCatalog(mongoClient, config.MongoConfig.MongoDBName)
	// Fail startup loudly if existing package codes are duplicated.
	// Existing duplicates must be reconciled manually before creating this index.
	if err := packageCatalog.EnsureIndexes(ctx); err != nil {
		logger.Fatal().Err(err).Msg("ensure unique loan product index")
	}
	composition.AttachLoanProducts(loanHandler, packageCatalog)
	composition.AttachLoanContractOTP(loanHandler, rdb, config.SMTPConfig, *logger)

	composition.AttachLoanRepayment(
		loanHandler,
		bank,
		loanRepo,
		mongoClient,
		config.MongoConfig.MongoDBName,
		rdb,
	)

	// An invalid savings catalog must fail CLOSED for savings without making
	// unrelated account, transfer and loan APIs unavailable. Operators must
	// repair the catalog and restart this instance before enabling Savings.
	savingsCatalog := mongo.NewSavingsPackageCatalog(mongoClient, config.MongoConfig.MongoDBName)
	savingsRead := mongo.NewSavingsRead(mongoClient, config.MongoConfig.MongoDBName)
	savingsHandler := composition.NewSavingsHandler(savingsCatalog, bank, rdb, savingsRead)
	if err := savingsRead.EnsureIndexes(ctx); err != nil {
		logger.Error().Err(err).Msg("savings read indexes unavailable; savings read API disabled")
		savingsHandler.Read = nil
	} else {
		go mongo.NewSavingsMaturityWorker(mongoClient, config.MongoConfig.MongoDBName, bank).Run(ctx)
	}
	if err := savingsCatalog.EnsureSeeded(ctx); err != nil {
		logger.Error().Err(err).Msg("savings catalog unavailable; savings APIs disabled (banking APIs remain up)")
		savingsHandler.Service = nil
		savingsHandler.Commands = nil
	}

	mux := netHTTP.NewServeMux()
	mux = presentation.NewRouterWithSavings(mux, accountHandler, mediaHandler, otpHandler, transactionHandler, tokenSvc, config.DocsConfig.SwaggerEnabled, savingsHandler, loanHandler)

	logger.Info().Msg("Anomaly Fraud Detection running on port :8080...")
	allowedOrigins := helpers.SplitCSV(loader.LoadEnvOr(
		"CORS_ALLOWED_ORIGINS",
		"http://localhost:1420,http://localhost:1422,http://localhost:5173,http://localhost:3000,tauri://localhost,http://tauri.localhost,https://tauri.localhost",
	))
	srv := &netHTTP.Server{
		Addr:              ":8080",
		Handler:           middleware.NewCORS(allowedOrigins)(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
