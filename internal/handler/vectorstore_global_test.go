package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/types"
)

type vectorStoreOptionsRepo struct {
	stores []*types.VectorStore
}

func (r *vectorStoreOptionsRepo) Create(context.Context, *types.VectorStore) error {
	return nil
}

func (r *vectorStoreOptionsRepo) GetByID(context.Context, uint64, string) (*types.VectorStore, error) {
	return nil, nil
}

func (r *vectorStoreOptionsRepo) List(context.Context, uint64) ([]*types.VectorStore, error) {
	return r.stores, nil
}

func (r *vectorStoreOptionsRepo) Update(context.Context, *types.VectorStore) error {
	return nil
}

func (r *vectorStoreOptionsRepo) UpdateConnectionConfig(context.Context, *types.VectorStore) error {
	return nil
}

func (r *vectorStoreOptionsRepo) Delete(context.Context, uint64, string) error {
	return nil
}

func (r *vectorStoreOptionsRepo) ExistsByEndpointAndIndex(
	context.Context,
	uint64,
	types.RetrieverEngineType,
	string,
	string,
) (bool, error) {
	return false, nil
}

func TestListStoreOptionsIsGlobalAndCredentialFree(t *testing.T) {
	t.Setenv("RETRIEVE_DRIVER", "")
	gin.SetMode(gin.TestMode)

	repo := &vectorStoreOptionsRepo{stores: []*types.VectorStore{{
		ID:         "store-1",
		Name:       "Shared Milvus",
		EngineType: types.MilvusRetrieverEngineType,
		ConnectionConfig: types.ConnectionConfig{
			Addr:     "milvus.internal:19530",
			Username: "root",
			Password: "secret",
		},
		IndexConfig: types.IndexConfig{CollectionName: "enterprise_chunks"},
	}}}
	h := NewVectorStoreHandler(repo, nil)

	router := gin.New()
	router.GET("/vector-stores/options", h.ListStoreOptions)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/vector-stores/options", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                `json:"success"`
		Data    []VectorStoreOption `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, []VectorStoreOption{{
		ID:         "store-1",
		Name:       "Shared Milvus",
		EngineType: string(types.MilvusRetrieverEngineType),
		Source:     types.StoreSourceUser,
		Status:     "available",
	}}, response.Data)

	body := recorder.Body.String()
	require.NotContains(t, body, "connection_config")
	require.NotContains(t, body, "index_config")
	require.NotContains(t, body, "milvus.internal")
	require.NotContains(t, body, "secret")
}
