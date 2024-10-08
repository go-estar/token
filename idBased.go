package token

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/go-estar/redis"
	"github.com/google/uuid"
	"time"
)

var (
	ErrorTokenCreateParams  = errors.New("id can't be empty")
	ErrorTokenVerifyParams  = errors.New("token can't be empty")
	ErrorTokenVerifyId      = errors.New("token verify failed")
	ErrorTokenVerifyToken   = errors.New("token verify failed")
	ErrorTokenVerifyChanged = errors.New("your account has login another device")
)

type IdBased struct {
	Redis      *redis.Redis
	Name       string
	ExpireTime int
	Repeatable bool //可以重复生成
	Disposable bool //一次性的
}

func NewIdBasedToken(id string) string {
	u4 := uuid.New().String()
	now := time.Now().String()
	hash := sha1.New()
	hash.Write([]byte(id + "," + now + "," + u4))
	token := hex.EncodeToString(hash.Sum(nil))
	return token
}

func (ib *IdBased) Get(id interface{}) (string, error) {
	idStr := fmt.Sprint(id)
	token, err := ib.Redis.Get(context.Background(), ib.Name+":"+idStr).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrorTokenVerifyToken
		}
		return "", err
	}
	return token, nil
}

func (ib *IdBased) Create(id interface{}, expireTime int) (t string, err error) {

	if id == nil {
		return "", ErrorTokenCreateParams
	}

	idStr := fmt.Sprint(id)

	var old string
	if !ib.Repeatable {
		old, err = ib.Redis.Get(context.Background(), ib.Name+":"+idStr).Result()
		if err != nil && err != redis.Nil {
			return "", err
		}
	}

	token := NewIdBasedToken(idStr)

	et := ib.ExpireTime
	if expireTime != 0 {
		et = expireTime
	}

	ctx := context.Background()
	multi := ib.Redis.TxPipeline()
	multi.Set(ctx, ib.Name+"-store:"+token, id, time.Duration(et)*time.Second)
	multi.Set(ctx, ib.Name+":"+idStr, token, time.Duration(et)*time.Second)
	if !ib.Repeatable && old != "" {
		multi.Del(ctx, ib.Name+"-store:"+old)
	}
	_, err = multi.Exec(ctx)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (ib *IdBased) Verify(token string) (id string, err error) {

	if token == "" {
		return "", ErrorTokenVerifyParams
	}

	if ib.Disposable {
		id, err = ib.Redis.GetDel(context.Background(), ib.Name+"-store:"+token)
	} else {
		id, err = ib.Redis.Get(context.Background(), ib.Name+"-store:"+token).Result()
	}

	if err != nil {
		if err == redis.Nil {
			return "", ErrorTokenVerifyId
		}
		return "", err
	}

	t, err := ib.Redis.Get(context.Background(), ib.Name+":"+id).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrorTokenVerifyToken
		}
		return "", err
	}

	if !ib.Repeatable && t != token {
		return "", ErrorTokenVerifyChanged
	}

	return id, nil
}
