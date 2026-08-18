package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/types"
)

type exchangeRateHandlerService struct {
	configured types.ExchangeRateConfig
}

func (s *exchangeRateHandlerService) GetExchangeRate(context.Context) (*types.ExchangeRate, error) {
	return &types.ExchangeRate{CurrencyPair: types.RMBCHFExchangeRatePair, RMBAmount: 6, CHFAmount: 1, IsDefault: true}, nil
}

func (s *exchangeRateHandlerService) ConfigureExchangeRate(
	_ context.Context, config types.ExchangeRateConfig, _ string,
) (*types.ExchangeRate, error) {
	s.configured = config
	return &types.ExchangeRate{CurrencyPair: types.RMBCHFExchangeRatePair, RMBAmount: config.RMBAmount, CHFAmount: config.CHFAmount}, nil
}

func TestExchangeRateHandlerGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/exchange-rate", NewExchangeRateHandler(&exchangeRateHandlerService{}).Get)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/exchange-rate", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"rmb_amount":6`) ||
		!strings.Contains(recorder.Body.String(), `"is_default":true`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestExchangeRateHandlerConfigure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &exchangeRateHandlerService{}
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.PUT("/exchange-rate/config", NewExchangeRateHandler(service).Configure)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/exchange-rate/config", strings.NewReader(`{"rmb_amount":6.25,"chf_amount":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.configured.RMBAmount != 6.25 ||
		!strings.Contains(recorder.Body.String(), `"message":"汇率配置保存成功"`) {
		t.Fatalf("status=%d body=%s configured=%+v", recorder.Code, recorder.Body.String(), service.configured)
	}
}

func TestExchangeRateHandlerRejectsZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.PUT("/exchange-rate/config", NewExchangeRateHandler(&exchangeRateHandlerService{}).Configure)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/exchange-rate/config", strings.NewReader(`{"rmb_amount":0,"chf_amount":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
