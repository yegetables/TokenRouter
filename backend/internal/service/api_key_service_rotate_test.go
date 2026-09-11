//go:build unit

// API Key 轮换的单元测试：验证所有权、托管 Key 拦截、配置保留、CAS 冲突与缓存失效。

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

// apiKeyRotateCall 记录一次 RotateKey 调用的入参，用于断言 CAS 语义。
type apiKeyRotateCall struct {
	id          int64
	expectedKey string
	newKey      string
}

// RotateKey 让 apiKeyRepoStub 满足 APIKeyCredentialRepository。
func (s *apiKeyRepoStub) RotateKey(ctx context.Context, id int64, expectedKey, newKey string) error {
	s.rotateCalls = append(s.rotateCalls, apiKeyRotateCall{id: id, expectedKey: expectedKey, newKey: newKey})
	return s.rotateErr
}

func newRotateTestService(repo *apiKeyRepoStub) (*APIKeyService, *apiKeyCacheStub) {
	cache := &apiKeyCacheStub{}
	return &APIKeyService{apiKeyRepo: repo, cache: cache, cfg: &config.Config{}}, cache
}

// 成功轮换：保留 ID 与全部配置，写入新 key，并同时失效新旧凭据缓存。
func TestAPIKeyService_Rotate_Success(t *testing.T) {
	groupID := int64(9)
	expiresAt := time.Now().Add(24 * time.Hour)
	repo := &apiKeyRepoStub{apiKey: &APIKey{
		ID: 42, UserID: 7, Key: "sk-old", Name: "prod", GroupID: &groupID,
		Quota: 12.5, ExpiresAt: &expiresAt,
	}}
	svc, cache := newRotateTestService(repo)

	got, err := svc.Rotate(context.Background(), 42, 7)
	require.NoError(t, err)
	require.Equal(t, int64(42), got.ID, "轮换必须保留 Key ID")
	require.NotEqual(t, "sk-old", got.Key, "凭据值必须变化")
	require.True(t, strings.HasPrefix(got.Key, "sk-"))
	require.Equal(t, "prod", got.Name)
	require.Equal(t, &groupID, got.GroupID)
	require.Equal(t, 12.5, got.Quota)
	require.Equal(t, &expiresAt, got.ExpiresAt)

	require.Len(t, repo.rotateCalls, 1)
	require.Equal(t, int64(42), repo.rotateCalls[0].id)
	require.Equal(t, "sk-old", repo.rotateCalls[0].expectedKey, "CAS 必须基于旧 key")
	require.Equal(t, got.Key, repo.rotateCalls[0].newKey)
	require.Equal(t, []string{svc.authCacheKey("sk-old"), svc.authCacheKey(got.Key)}, cache.deleteAuthKeys)
}

// 非所有者不能轮换。
func TestAPIKeyService_Rotate_OwnerMismatch(t *testing.T) {
	repo := &apiKeyRepoStub{apiKey: &APIKey{ID: 42, UserID: 1, Key: "sk-old"}}
	svc, cache := newRotateTestService(repo)

	_, err := svc.Rotate(context.Background(), 42, 2)
	require.ErrorIs(t, err, ErrInsufficientPerms)
	require.Empty(t, repo.rotateCalls)
	require.Empty(t, cache.deleteAuthKeys)
}

// 服务端托管 Key（如创作台隐藏 Key）不可轮换，且不暴露存在性。
func TestAPIKeyService_Rotate_ManagedKeyHidden(t *testing.T) {
	managed := "creative_studio"
	repo := &apiKeyRepoStub{apiKey: &APIKey{ID: 42, UserID: 7, Key: "sk-old", ManagedBy: &managed}}
	svc, _ := newRotateTestService(repo)

	_, err := svc.Rotate(context.Background(), 42, 7)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Empty(t, repo.rotateCalls)
}

// 记录不存在时透传 NotFound。
func TestAPIKeyService_Rotate_NotFound(t *testing.T) {
	repo := &apiKeyRepoStub{getByIDErr: ErrAPIKeyNotFound}
	svc, _ := newRotateTestService(repo)

	_, err := svc.Rotate(context.Background(), 42, 7)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Empty(t, repo.rotateCalls)
}

// 并发轮换冲突时透传 Conflict，且不生成第二个成功结果。
func TestAPIKeyService_Rotate_Conflict(t *testing.T) {
	repo := &apiKeyRepoStub{
		apiKey:    &APIKey{ID: 42, UserID: 7, Key: "sk-old"},
		rotateErr: ErrAPIKeyRotateConflict,
	}
	svc, _ := newRotateTestService(repo)

	_, err := svc.Rotate(context.Background(), 42, 7)
	require.ErrorIs(t, err, ErrAPIKeyRotateConflict)
	require.Len(t, repo.rotateCalls, 1, "CAS 尝试应发生，但返回冲突")
}
