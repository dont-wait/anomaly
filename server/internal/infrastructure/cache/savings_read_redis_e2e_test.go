//go:build savings_e2e

package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestSavingsReadRedisCacheTTLWithDocker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "redis:7-alpine", ExposedPorts: []string{"6379/tcp"}, WaitingFor: wait.ForLog("Ready to accept connections").WithStartupTimeout(time.Minute),
		}, Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, c)
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := c.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%s", host, port.Port())})
	defer redisClient.Close()
	store := NewSavingsReadRedis(redisClient)
	if err := store.Set(ctx, "test:savings:read", []byte("one"), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Get(ctx, "test:savings:read")
	if err != nil || !found || string(got) != "one" {
		t.Fatalf("cache miss: %v %v %s", err, found, got)
	}
	time.Sleep(170 * time.Millisecond)
	_, found, err = store.Get(ctx, "test:savings:read")
	if err != nil || found {
		t.Fatalf("expired key still present: found=%v err=%v", found, err)
	}
}
