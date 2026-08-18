package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/handler"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockAssetService struct{ mock.Mock }

func (m *mockAssetService) Create(input service.CreateAssetInput) (*domain.Asset, error) {
	args := m.Called(input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Asset), args.Error(1)
}
func (m *mockAssetService) GetByID(id uint) (*domain.Asset, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Asset), args.Error(1)
}
func (m *mockAssetService) Update(id uint, input service.CreateAssetInput) (*domain.Asset, error) {
	args := m.Called(id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Asset), args.Error(1)
}
func (m *mockAssetService) Delete(id uint) error { return m.Called(id).Error(0) }
func (m *mockAssetService) DeleteAll() error     { return m.Called().Error(0) }
func (m *mockAssetService) ListAll() ([]domain.Asset, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Asset), args.Error(1)
}
func (m *mockAssetService) ListSince(since time.Time) ([]domain.Asset, error) {
	args := m.Called(since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Asset), args.Error(1)
}
func (m *mockAssetService) GetSummary() (*domain.AssetSummary, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AssetSummary), args.Error(1)
}

func setupAssetRouter(svc service.AssetService) *gin.Engine {
	r := gin.New()
	handler.NewAssetHandler(svc).RegisterRoutes(r.Group("/api/v1"))
	return r
}

func TestAssetHandler_Create(t *testing.T) {
	t.Run("201 on valid asset", func(t *testing.T) {
		svc := &mockAssetService{}
		r := setupAssetRouter(svc)

		input := service.CreateAssetInput{
			Name:          "Toyota RAV4 Style Hybrid",
			Type:          domain.AssetTypeVehicle,
			PurchaseDate:  "2020-11-17",
			PurchasePrice: 34000,
		}
		svc.On("Create", input).Return(&domain.Asset{ID: 1, Name: input.Name, Type: input.Type, PurchasePrice: 34000, CurrentValue: 34000}, nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var a domain.Asset
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &a))
		assert.Equal(t, "Toyota RAV4 Style Hybrid", a.Name)
		svc.AssertExpectations(t)
	})

	t.Run("400 on missing required fields", func(t *testing.T) {
		svc := &mockAssetService{}
		r := setupAssetRouter(svc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		svc.AssertNotCalled(t, "Create", mock.Anything)
	})

	t.Run("400 on invalid asset type", func(t *testing.T) {
		svc := &mockAssetService{}
		r := setupAssetRouter(svc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/assets",
			bytes.NewReader([]byte(`{"name":"X","type":"spaceship","purchase_price":100}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAssetHandler_ListAll(t *testing.T) {
	svc := &mockAssetService{}
	r := setupAssetRouter(svc)

	svc.On("ListAll").Return([]domain.Asset{
		{ID: 1, Name: "RAV4"},
		{ID: 2, Name: "House"},
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	var assets []domain.Asset
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &assets))
	assert.Len(t, assets, 2)
}

func TestAssetHandler_GetSummary(t *testing.T) {
	svc := &mockAssetService{}
	r := setupAssetRouter(svc)

	svc.On("GetSummary").Return(&domain.AssetSummary{
		Count: 3, TotalValue: 448551.50, TotalLoans: 263568.67, NetEquity: 184982.83,
	}, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/assets/summary", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	var s domain.AssetSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
	assert.Equal(t, 3, s.Count)
	assert.InDelta(t, 184982.83, s.NetEquity, 0.001)
}

func TestAssetHandler_GetByID(t *testing.T) {
	t.Run("200 when found", func(t *testing.T) {
		svc := &mockAssetService{}
		r := setupAssetRouter(svc)
		svc.On("GetByID", uint(1)).Return(&domain.Asset{ID: 1, Name: "House"}, nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/assets/1", nil))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("404 when missing", func(t *testing.T) {
		svc := &mockAssetService{}
		r := setupAssetRouter(svc)
		svc.On("GetByID", uint(9)).Return(nil, errors.New("not found"))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/assets/9", nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("400 on non-numeric id", func(t *testing.T) {
		svc := &mockAssetService{}
		r := setupAssetRouter(svc)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/assets/abc", nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAssetHandler_Update(t *testing.T) {
	svc := &mockAssetService{}
	r := setupAssetRouter(svc)

	input := service.CreateAssetInput{
		Name:          "House Platiniškių 21A",
		Type:          domain.AssetTypeRealEstate,
		PurchasePrice: 355000,
		CurrentValue:  410000,
	}
	svc.On("Update", uint(2), input).Return(&domain.Asset{ID: 2, Name: input.Name, CurrentValue: 410000}, nil)

	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/2", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var a domain.Asset
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &a))
	assert.Equal(t, 410000.0, a.CurrentValue)
}

func TestAssetHandler_Delete(t *testing.T) {
	t.Run("204 on success", func(t *testing.T) {
		svc := &mockAssetService{}
		r := setupAssetRouter(svc)
		svc.On("Delete", uint(1)).Return(nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/assets/1", nil))
		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("404 when missing", func(t *testing.T) {
		svc := &mockAssetService{}
		r := setupAssetRouter(svc)
		svc.On("Delete", uint(9)).Return(errors.New("not found"))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/assets/9", nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAssetHandler_DeleteAll(t *testing.T) {
	svc := &mockAssetService{}
	r := setupAssetRouter(svc)
	svc.On("DeleteAll").Return(nil)

	// Without the explicit confirmation the wipe is refused.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/assets", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	svc.AssertNotCalled(t, "DeleteAll")

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/assets?confirm=all", nil))
	assert.Equal(t, http.StatusNoContent, w.Code)
}
