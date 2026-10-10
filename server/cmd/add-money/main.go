package main

import (
	"context"
	"flag"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
)

func main() {
	accountNo := flag.String("account-no", "", "target account number")
	amount := flag.Int64("amount", 0, "amount to add in VND")
	flag.Parse()

	log := logger.NewLogger(zerolog.InfoLevel)
	if strings.TrimSpace(*accountNo) == "" {
		log.Fatal().Msg("-account-no is required")
	}
	if *amount <= 0 {
		log.Fatal().Msg("-amount must be a positive integer")
	}

	loader := domain.GetEnvLoader().Load(log)
	config := loader.LoadEventStoreConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := eventstore.NewEventStoreClient(config)
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore failed")
	}
	defer eventstore.Disconnect(client)

	bank := eventstore.NewBankRepository(client, nil)
	if err := bank.AddSeedBalanceByAccountNo(ctx, *accountNo, *amount); err != nil {
		log.Fatal().Err(err).Msg("add money failed")
	}
	log.Info().Str("account_no", strings.TrimSpace(*accountNo)).Int64("amount", *amount).Msg("money added")
}
