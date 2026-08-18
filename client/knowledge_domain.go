package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type KnowledgeDomain struct {
	ID           uint64    `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	NameEn       string    `json:"name_en"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	StorageQuota int64     `json:"storage_quota"`
	StorageUsed  int64     `json:"storage_used"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type KnowledgeDomainResponse struct {
	Success bool            `json:"success"`
	Data    KnowledgeDomain `json:"data"`
}

type KnowledgeDomainListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items []KnowledgeDomain `json:"items"`
	} `json:"data"`
}

func (c *Client) CreateKnowledgeDomain(ctx context.Context, domain *KnowledgeDomain) (*KnowledgeDomain, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/knowledge-domains", domain, nil)
	if err != nil {
		return nil, err
	}
	var response KnowledgeDomainResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

func (c *Client) GetKnowledgeDomain(ctx context.Context, id uint64) (*KnowledgeDomain, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/knowledge-domains/%d", id), nil, nil)
	if err != nil {
		return nil, err
	}
	var response KnowledgeDomainResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

func (c *Client) UpdateKnowledgeDomain(ctx context.Context, domain *KnowledgeDomain) (*KnowledgeDomain, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/knowledge-domains/%d", domain.ID), domain, nil)
	if err != nil {
		return nil, err
	}
	var response KnowledgeDomainResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, err
	}
	return &response.Data, nil
}

func (c *Client) DeleteKnowledgeDomain(ctx context.Context, id uint64) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/knowledge-domains/%d", id), nil, nil)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

func (c *Client) ListKnowledgeDomains(ctx context.Context) ([]KnowledgeDomain, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/knowledge-domains", nil, nil)
	if err != nil {
		return nil, err
	}
	var response KnowledgeDomainListResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, err
	}
	return response.Data.Items, nil
}

type KnowledgeDomainSearchResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items    []KnowledgeDomain `json:"items"`
		Total    int64             `json:"total"`
		Page     int               `json:"page"`
		PageSize int               `json:"page_size"`
	} `json:"data"`
}

func (c *Client) SearchKnowledgeDomains(
	ctx context.Context, keyword string, knowledgeDomainID uint64, page, pageSize int,
) ([]KnowledgeDomain, int64, error) {
	query := url.Values{
		"page":      {strconv.Itoa(page)},
		"page_size": {strconv.Itoa(pageSize)},
	}
	if keyword != "" {
		query.Set("keyword", keyword)
	}
	if knowledgeDomainID > 0 {
		query.Set("knowledge_domain_id", strconv.FormatUint(knowledgeDomainID, 10))
	}
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/knowledge-domains/search", nil, query)
	if err != nil {
		return nil, 0, err
	}
	var response KnowledgeDomainSearchResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, 0, err
	}
	return response.Data.Items, response.Data.Total, nil
}

func (c *Client) GetPlatformRuntimeConfig(ctx context.Context, key string) (json.RawMessage, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/system/runtime-config/%s", key), nil, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (c *Client) UpdatePlatformRuntimeConfig(ctx context.Context, key string, value any) (json.RawMessage, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/system/runtime-config/%s", key), value, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
