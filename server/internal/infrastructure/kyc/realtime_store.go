package kyc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/redis/go-redis/v9"
)

var (
	ErrAttemptConflict = errors.New("KYC attempt conflict")
	ErrAttemptLimit    = errors.New("KYC attempts exhausted")
	ErrAttemptCapacity = errors.New("KYC capacity or start rate exceeded")
)

type Attempt struct {
	ID         string          `json:"id"`
	AccountID  string          `json:"account_id"`
	Key        string          `json:"key"`
	Front      Media           `json:"front"`
	Back       Media           `json:"back"`
	FrontHash  string          `json:"front_hash"`
	BackHash   string          `json:"back_hash"`
	Challenge  string          `json:"challenge"`
	ExpiresAt  time.Time       `json:"expires_at"`
	StreamHash string          `json:"stream_hash"`
	Result     json.RawMessage `json:"result,omitempty"`
}

// Redis owns attempt policy across API replicas and KYC-token renewal.
// Media is short-lived and private; Redis must be configured with private access.
type Policy struct {
	MaxAttempts     int
	Window          time.Duration
	AttemptTTL      time.Duration
	MaxConcurrent   int
	StartsPerMinute int
}

func DefaultPolicy() Policy {
	return Policy{MaxAttempts: 3, Window: 24 * time.Hour, AttemptTTL: 2 * time.Minute, MaxConcurrent: 8, StartsPerMinute: 6}
}

type RealtimeStore struct {
	DB     redis.UniversalClient
	Policy Policy
}

func (s *RealtimeStore) Limits() Policy {
	if s.Policy.MaxAttempts <= 0 {
		return DefaultPolicy()
	}
	return s.Policy
}

func Digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func (s *RealtimeStore) Get(ctx context.Context, id string) (*Attempt, error) {
	data, err := s.DB.Get(ctx, "kyc:attempt:"+id).Bytes()
	if err != nil {
		return nil, err
	}
	var a Attempt
	err = json.Unmarshal(data, &a)
	return &a, err
}

func (s *RealtimeStore) Save(ctx context.Context, a *Attempt) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	ttl := time.Until(a.ExpiresAt)
	if len(a.Result) > 0 {
		ttl = s.Limits().Window
	}
	if ttl <= 0 {
		return ErrAttemptConflict
	}
	stored, err := s.DB.Eval(ctx, `if redis.call('EXISTS',KEYS[2])==1 then return 0 end redis.call('SET',KEYS[1],ARGV[1],'PX',ARGV[2]);return 1`, []string{"kyc:attempt:" + a.ID, "kyc:cancelled:" + a.ID}, string(data), ttl.Milliseconds()).Int()
	if err != nil {
		return err
	}
	if stored == 0 {
		return ErrAttemptConflict
	}
	return nil
}

func (s *RealtimeStore) Create(ctx context.Context, a *Attempt) (*Attempt, error) {
	a.ID = Digest([]byte(a.AccountID + ":" + a.Key))[:40]
	if previous, err := s.Get(ctx, a.ID); err == nil {
		if previous.FrontHash != a.FrontHash || previous.BackHash != a.BackHash || previous.Challenge != a.Challenge {
			return nil, ErrAttemptConflict
		}
		return previous, nil
	} else if !errors.Is(err, redis.Nil) {
		return nil, err
	}
	data, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	p := s.Limits()
	status, err := s.DB.Eval(ctx, `
 if redis.call('EXISTS',KEYS[1])==1 then return 1 end
 if tonumber(redis.call('GET',KEYS[2]) or '0')>=tonumber(ARGV[4]) then return 2 end
 if redis.call('EXISTS',KEYS[3])==1 then return 3 end
 local rate=redis.call('INCR',KEYS[4]);if rate==1 then redis.call('EXPIRE',KEYS[4],60) end
 if rate>tonumber(ARGV[5]) then return 4 end
 redis.call('ZREMRANGEBYSCORE',KEYS[5],'-inf',ARGV[6])
 if redis.call('ZCARD',KEYS[5])>=tonumber(ARGV[7]) then return 4 end
 redis.call('SET',KEYS[1],ARGV[1],'PX',ARGV[2])
 redis.call('SET',KEYS[3],ARGV[3],'PX',ARGV[2])
 redis.call('ZADD',KEYS[5],ARGV[8],ARGV[3])
 return 0`, []string{"kyc:attempt:" + a.ID, "kyc:count:" + a.AccountID, "kyc:active:" + a.AccountID, "kyc:rate:" + a.AccountID, "kyc:leases"},
		string(data), time.Until(a.ExpiresAt).Milliseconds(), a.ID, p.MaxAttempts, p.StartsPerMinute, time.Now().UnixMilli(), p.MaxConcurrent, a.ExpiresAt.UnixMilli()).Int()
	if err != nil {
		return nil, err
	}
	switch status {
	case 1:
		previous, e := s.Get(ctx, a.ID)
		if e != nil {
			return nil, e
		}
		if previous.FrontHash != a.FrontHash || previous.BackHash != a.BackHash || previous.Challenge != a.Challenge {
			return nil, ErrAttemptConflict
		}
		return previous, nil
	case 2:
		return nil, ErrAttemptLimit
	case 3:
		return nil, ErrAttemptConflict
	case 4:
		return nil, ErrAttemptCapacity
	}
	return a, nil
}

func (s *RealtimeStore) Release(ctx context.Context, a *Attempt) {
	s.DB.Eval(ctx, `redis.call('ZREM',KEYS[2],ARGV[1]); if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{"kyc:active:" + a.AccountID, "kyc:leases"}, a.ID)
}

func (s *RealtimeStore) Ticket(ctx context.Context, a *Attempt, ticket string) error {
	if time.Now().After(a.ExpiresAt) {
		return ErrAttemptConflict
	}
	return s.DB.Set(ctx, "kyc:ticket:"+Digest([]byte(ticket)), a.ID, 30*time.Second).Err()
}

func (s *RealtimeStore) ConsumeTicket(ctx context.Context, id, ticket string) error {
	value, err := s.DB.GetDel(ctx, "kyc:ticket:"+Digest([]byte(ticket))).Result()
	if err != nil {
		return err
	}
	if value != id {
		return ErrAttemptConflict
	}
	return nil
}

func (s *RealtimeStore) Lock(ctx context.Context, id string) (string, error) {
	token := uuid.NewString()
	ok, err := s.DB.SetNX(ctx, "kyc:processing:"+id, token, 90*time.Second).Result()
	if !ok {
		return "", err
	}
	return token, err
}

func (s *RealtimeStore) Unlock(ctx context.Context, id, token string) {
	s.DB.Eval(ctx, `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{"kyc:processing:" + id}, token)
}

func (s *RealtimeStore) Count(ctx context.Context, account string) (int, time.Duration, error) {
	n, err := s.DB.Get(ctx, "kyc:count:"+account).Int()
	if errors.Is(err, redis.Nil) {
		err = nil
	}
	ttl, e := s.DB.TTL(ctx, "kyc:count:"+account).Result()
	if err == nil {
		err = e
	}
	return n, ttl, err
}

// CompleteFailure commits the result and its single policy debit together.
// A retry after a lost response reads this record without inference or another debit.
func (s *RealtimeStore) CompleteFailure(ctx context.Context, a *Attempt, result json.RawMessage, retryAfter string) (json.RawMessage, error) {
	record := *a
	record.Front.Data = nil
	record.Back.Data = nil
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	p := s.Limits()
	saved, err := s.DB.Eval(ctx, `
 local previous=redis.call('GET',KEYS[1]);if not previous then return redis.error_reply('attempt expired') end
 local prior=cjson.decode(previous);if prior.result then return cjson.encode(prior.result) end
 if redis.call('EXISTS',KEYS[4])==1 then return redis.error_reply('attempt cancelled') end
 local n=tonumber(redis.call('GET',KEYS[2]) or '0')
 if redis.call('SET',KEYS[3],'1','NX','PX',ARGV[3]) then
  n=redis.call('INCR',KEYS[2]);if n==1 then redis.call('PEXPIRE',KEYS[2],ARGV[3]) end
 end
 local result=cjson.decode(ARGV[2]);result.attemptsRemaining=math.max(0,tonumber(ARGV[4])-n);result.attemptLimit=tonumber(ARGV[4])
 if result.attemptsRemaining==0 then result.decision='FAILED_FINAL';result.retryAfter=ARGV[5] else result.retryAfter=nil end
 local record=cjson.decode(ARGV[1]);record.result=result
 redis.call('SET',KEYS[1],cjson.encode(record),'PX',ARGV[3])
 if redis.call('GET',KEYS[5])==ARGV[6] then redis.call('DEL',KEYS[5]) end
 redis.call('ZREM',KEYS[6],ARGV[6])
 return cjson.encode(result)`, []string{"kyc:attempt:" + a.ID, "kyc:count:" + a.AccountID, "kyc:counted:" + a.ID, "kyc:cancelled:" + a.ID, "kyc:active:" + a.AccountID, "kyc:leases"},
		string(encoded), string(result), max(1, p.Window.Milliseconds()), p.MaxAttempts, retryAfter, a.ID).Text()
	if err != nil {
		return nil, err
	}
	return json.RawMessage(saved), nil
}
