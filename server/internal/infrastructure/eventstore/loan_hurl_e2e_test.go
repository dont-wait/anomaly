//go:build e2e

package eventstore_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

func runLoanHurlE2E(t *testing.T, parent context.Context, baseURL string, es *esdb.Client, mongoClient *mongodrv.Client, dbName string) {
	t.Helper()
	hurl, err := exec.LookPath("hurl")
	if err != nil {
		t.Fatal("test-loan-hurl-e2e requires hurl on PATH")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	// Hurl chỉ giao tiếp qua HTTP. Project dữ liệu bất đồng bộ bằng các adapter
	// thật để các GET có retry kiểm tra được eventual consistency; không chèn
	// sẵn feed/read model hoặc sửa response cho bài test. Vòng lặp này không
	// thay thế test checkpoint/restart của các worker binary.
	sub, err := es.SubscribeToAll(ctx, esdb.SubscribeToAllOptions{From: esdb.Start{}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- projectLoanHurlEvents(ctx, sub,
			mongorepo.NewBankProjection(mongoClient, dbName),
			eventstore.NewLoanRepository(es), mongorepo.NewLoanReadModel(mongoClient, dbName))
	}()
	defer func() {
		cancel()
		_ = sub.Close()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Hurl projection: %v", err)
		}
	}()

	// Hurl suy luận kiểu biến CLI; dấu nháy nằm trong giá trị giữ số 0 đầu CCCD.
	cmd := exec.CommandContext(ctx, hurl,
		"--test", "--no-color", "--connect-timeout", "5", "--max-time", "10",
		"--variable", "base_url="+baseURL,
		"--variable", `customer_cccd="000000000001"`, "--secret", "customer_password=TestBank@123",
		"--variable", `admin_cccd="000000000002"`, "--secret", "admin_password=TestBank@123",
		"../../../tests/api/loans/lifecycle.hurl")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("loan Hurl suite: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}

func projectLoanHurlEvents(ctx context.Context, sub *esdb.Subscription, bank *mongorepo.BankProjection, loans *eventstore.LoanRepository, read *mongorepo.LoanReadModel) error {
	for ctx.Err() == nil {
		message := sub.Recv()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if message.SubscriptionDropped != nil {
			return fmt.Errorf("subscription dropped: %w", message.SubscriptionDropped.Error)
		}
		if message.EventAppeared == nil {
			continue
		}
		recorded := message.EventAppeared.Event
		switch {
		case recorded.StreamID == journal.BankStream:
			record, err := journal.DecodeRecord(recorded.Data, recorded.EventID.String(), recorded.EventType)
			if err != nil {
				return err
			}
			if err := bank.Apply(ctx, record, recorded.EventNumber); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
		case strings.HasPrefix(recorded.StreamID, "loan-application-"):
			app, revision, err := loans.FindByIDWithRevision(ctx, strings.TrimPrefix(recorded.StreamID, "loan-application-"))
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			if app == nil {
				return fmt.Errorf("loan stream %s has no application", recorded.StreamID)
			}
			if err := read.Save(ctx, app, revision); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
		}
	}
	return ctx.Err()
}
