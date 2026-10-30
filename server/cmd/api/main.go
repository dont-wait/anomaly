package main

import (
	"context"
	netHTTP "net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/dont-wait/anomaly/internal/application/riskstream"
	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	"github.com/dont-wait/anomaly/internal/helpers"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/kyc"
	"github.com/dont-wait/anomaly/internal/infrastructure/loanadapter"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	rustfs "github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/logger"
	presentation "github.com/dont-wait/anomaly/internal/presentation/http"
	adminhandler "github.com/dont-wait/anomaly/internal/presentation/http/handler/admin"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := logger.NewLogger(zerolog.InfoLevel)

	loader := domain.GetEnvLoader().Load(logger)
	config := loader.LoadAllConfig()
	if strings.TrimSpace(config.KafkaConfig.RiskDecisionSigningKey) == "" {
		logger.Fatal().Msg("KAFKA_RISK_DECISION_SIGNING_KEY is required for model responses")
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
	riskAssessments := mongo.NewRiskAssessmentRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := riskAssessments.EnsureIndexes(ctx); err != nil {
		logger.Fatal().Err(err).Msg("ensure risk assessment indexes failed")
	}
	riskHub := riskstream.NewHub()
	go func() {
		var resumeToken bson.Raw
		var startAtOperationTime *bson.Timestamp
		var lastReceivedAt time.Time
		var lastAssessmentID string
		var lastCursor int64
		publish := func(assessment riskassessment.Assessment) {
			if lastCursor > 0 && assessment.Cursor > 0 && assessment.Cursor <= lastCursor {
				return
			}
			if assessment.Cursor == 0 && !lastReceivedAt.IsZero() {
				if assessment.ReceivedAt.Before(lastReceivedAt) || (assessment.ReceivedAt.Equal(lastReceivedAt) && assessment.ID <= lastAssessmentID) {
					return
				}
			}
			if assessment.Cursor > 0 {
				lastCursor = assessment.Cursor
			}
			lastReceivedAt = assessment.ReceivedAt
			lastAssessmentID = assessment.ID
			riskHub.Publish(riskstream.Event{ID: assessment.ID, Assessment: assessment})
		}
		for ctx.Err() == nil {
			nextToken, err := riskAssessments.Watch(ctx, resumeToken, startAtOperationTime, publish)
			if len(nextToken) > 0 {
				resumeToken = append(bson.Raw(nil), nextToken...)
			}
			if ctx.Err() != nil {
				return
			}
			var recoveryStart *bson.Timestamp
			if isChangeStreamHistoryLost(err) {
				now := uint32(time.Now().Unix())
				recoveryStart = &bson.Timestamp{T: now}
			}
			if replayErr := replayRiskAssessments(ctx, riskAssessments, publish, lastCursor, lastReceivedAt, lastAssessmentID); replayErr != nil {
				logger.Error().Err(replayErr).Msg("risk assessment recovery replay failed")
			}
			if isChangeStreamHistoryLost(err) {
				resumeToken = nil
				startAtOperationTime = recoveryStart
			} else if len(nextToken) > 0 {
				startAtOperationTime = nil
			}
			logger.Error().Err(err).Msg("risk assessment change stream stopped; retrying")
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	var statusMu sync.Mutex
	lastStatusFingerprints := make(map[string]string)
	publishStatus := func(status mongo.RiskTransactionStatus) {
		fingerprint := transactionStatusFingerprint(status)
		statusMu.Lock()
		if lastStatusFingerprints[status.TransactionID] == fingerprint {
			statusMu.Unlock()
			return
		}
		lastStatusFingerprints[status.TransactionID] = fingerprint
		statusMu.Unlock()
		riskHub.Publish(riskstream.Event{ID: status.TransactionID, TransactionStatus: &riskstream.TransactionStatus{
			TransactionID: status.TransactionID,
			Status:        status.Status,
			StatusAt:      status.StatusAt,
			OTPExpiresAt:  status.OTPExpiresAt,
			OTPVerifiedAt: status.OTPVerifiedAt,
			CompletedAt:   status.CompletedAt,
			FailedAt:      status.FailedAt,
			CancelledAt:   status.CancelledAt,
		}})
	}
	go func() {
		var resumeToken bson.Raw
		var startAtOperationTime *bson.Timestamp
		for ctx.Err() == nil {
			nextToken, err := riskAssessments.WatchTransactionStatuses(ctx, resumeToken, startAtOperationTime, publishStatus)
			if len(nextToken) > 0 {
				resumeToken = append(bson.Raw(nil), nextToken...)
				startAtOperationTime = nil
			}
			if ctx.Err() != nil {
				return
			}
			if isChangeStreamHistoryLost(err) {
				resumeToken = nil
				startAtOperationTime = &bson.Timestamp{T: uint32(time.Now().Unix())}
			}
			logger.Error().Err(err).Msg("transaction status change stream stopped; retrying")
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				statuses, err := riskAssessments.ListExpiredTransactionStatuses(ctx)
				if err != nil {
					logger.Error().Err(err).Msg("list expired transaction statuses failed")
					continue
				}
				for _, status := range statuses {
					publishStatus(status)
				}
			}
		}
	}()

	esClient, err := eventstore.NewEventStoreClient(config.EventStoreConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect eventstore failed")
	}
	defer eventstore.Disconnect(esClient)
	transferRepository := mongo.NewTransferRepository(mongoClient, config.MongoConfig.MongoDBName)
	bank := eventstore.NewBankRepository(esClient, transferRepository)
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

	accountHandler := composition.NewAccountHandlerWithKYC(bank, tokenSvc, kyc.NewClient(config.KYCConfig.ServiceURL, config.KYCConfig.ServiceToken).WithPOCBypass(config.KYCConfig.POCBypass), mediaRepo, *logger)
	adminMonitorHandler := adminhandler.NewMonitorHandler(riskAssessments, riskHub, transferRepository, accountHandler.IsCurrentAdmin)
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
	transactionHandler.SetRiskDecisionStore(riskAssessments)
	go composition.NewReceiptWorker(receiptOutbox, config.SMTPConfig, *logger).Run(ctx)
	go runRiskDecisionConsumer(ctx, config.KafkaConfig, riskAssessments, *logger)

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
	mux = presentation.NewRouterWithSavingsAndAdminMonitor(
		mux, accountHandler, mediaHandler, otpHandler, transactionHandler,
		tokenSvc, config.DocsConfig.SwaggerEnabled,
		savingsHandler, adminMonitorHandler, loanHandler,
	)

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
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.ListenAndServe() }()
	select {
	case err := <-serverErr:
		if err != nil && err != netHTTP.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server failed")
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		logger.Info().Msg("shutdown signal received; draining HTTP and background workers")
		riskHub.Close()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("graceful HTTP shutdown failed")
		}
	}
}

func replayRiskAssessments(ctx context.Context, repository *mongo.RiskAssessmentRepository, publish func(riskassessment.Assessment), afterCursor int64, after time.Time, afterID string) error {
	if afterCursor == 0 && after.IsZero() {
		return nil
	}
	for {
		items, err := repository.ListAfter(ctx, afterCursor, after, afterID, 1000)
		if err != nil {
			return err
		}
		for _, item := range items {
			publish(item)
			afterCursor = item.Cursor
			after, afterID = item.ReceivedAt, item.ID
		}
		if len(items) < 1000 {
			return nil
		}
	}
}

func isChangeStreamHistoryLost(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "changestreamhistorylost") || strings.Contains(message, "resume token")
}

func transactionStatusFingerprint(status mongo.RiskTransactionStatus) string {
	return strings.Join([]string{
		status.Status,
		status.StatusAt.UTC().Format(time.RFC3339Nano),
		timeString(status.OTPExpiresAt),
		timeString(status.OTPVerifiedAt),
		timeString(status.CompletedAt),
		timeString(status.FailedAt),
		timeString(status.CancelledAt),
	}, "|")
}

func timeString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
